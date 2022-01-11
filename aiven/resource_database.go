// Copyright (c) 2017 jelmersnoeck
// Copyright (c) 2018-2021 Aiven, Helsinki, Finland. https://aiven.io/
package aiven

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aiven/aiven-go-client"
	"github.com/aiven/terraform-provider-aiven/aiven/internal/schemautil"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

const defaultLC = "en_US.UTF-8"

// handleLcDefaults checks if the lc values have actually changed
func handleLcDefaults(_, old, new string, _ *schema.ResourceData) bool {
	// NOTE! not all database resources return lc_* values even if
	// they are set when the database is created; best we can do is
	// to assume it was created using the default value.
	return new == "" || (old == "" && new == defaultLC) || old == new
}

var aivenDatabaseSchema = map[string]*schema.Schema{
	"project":      commonSchemaProjectReference,
	"service_name": commonSchemaServiceNameReference,
	"database_name": {
		Type:        schema.TypeString,
		Required:    true,
		ForceNew:    true,
		Description: complex("The name of the service database.").forceNew().build(),
	},
	"lc_collate": {
		Type:             schema.TypeString,
		Optional:         true,
		Default:          defaultLC,
		ForceNew:         true,
		DiffSuppressFunc: handleLcDefaults,
		Description:      complex("Default string sort order (`LC_COLLATE`) of the database.").defaultValue(defaultLC).forceNew().build(),
	},
	"lc_ctype": {
		Type:             schema.TypeString,
		Optional:         true,
		Default:          defaultLC,
		ForceNew:         true,
		DiffSuppressFunc: handleLcDefaults,
		Description:      complex("Default character classification (`LC_CTYPE`) of the database.").defaultValue(defaultLC).forceNew().build(),
	},
	"termination_protection": {
		Type:        schema.TypeBool,
		Optional:    true,
		Default:     false,
		Description: complex(`It is a Terraform client-side deletion protections, which prevents the database from being deleted by Terraform. It is recommended to enable this for any production databases containing critical data.`).defaultValue(false).build(),
	},
}

func resourceDatabase() *schema.Resource {
	return &schema.Resource{
		Description:   "The Database resource allows the creation and management of Aiven Databases.",
		CreateContext: resourceDatabaseCreate,
		ReadContext:   resourceDatabaseRead,
		DeleteContext: resourceDatabaseDelete,
		UpdateContext: resourceDatabaseUpdate,
		Importer: &schema.ResourceImporter{
			StateContext: resourceDatabaseState,
		},
		Timeouts: &schema.ResourceTimeout{
			Delete: schema.DefaultTimeout(2 * time.Minute),
		},

		// TODO: add user config
		Schema: aivenDatabaseSchema,
	}
}

func resourceDatabaseCreate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*aiven.Client)

	projectName := d.Get("project").(string)
	serviceName := d.Get("service_name").(string)
	databaseName := d.Get("database_name").(string)
	_, err := client.Databases.Create(
		projectName,
		serviceName,
		aiven.CreateDatabaseRequest{
			Database:  databaseName,
			LcCollate: schemautil.OptionalString(d, "lc_collate"),
			LcType:    schemautil.OptionalString(d, "lc_ctype"),
		},
	)
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(schemautil.BuildResourceID(projectName, serviceName, databaseName))

	return resourceDatabaseRead(ctx, d, m)
}

func resourceDatabaseUpdate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	return resourceDatabaseRead(ctx, d, m)
}

func resourceDatabaseRead(_ context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*aiven.Client)

	projectName, serviceName, databaseName := schemautil.SplitResourceID3(d.Id())
	database, err := client.Databases.Get(projectName, serviceName, databaseName)
	if err != nil {
		return diag.FromErr(resourceReadHandleNotFound(err, d))
	}

	if err := d.Set("database_name", database.DatabaseName); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("project", projectName); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("service_name", serviceName); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("lc_collate", database.LcCollate); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("lc_ctype", database.LcType); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("termination_protection", d.Get("termination_protection")); err != nil {
		return diag.FromErr(err)
	}

	return nil
}

func resourceDatabaseDelete(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*aiven.Client)

	projectName, serviceName, databaseName := schemautil.SplitResourceID3(d.Id())

	if d.Get("termination_protection").(bool) {
		return diag.Errorf("cannot delete a database termination_protection is enabled")
	}

	waiter := DatabaseDeleteWaiter{
		Client:      client,
		ProjectName: projectName,
		ServiceName: serviceName,
		Database:    databaseName,
	}

	timeout := d.Timeout(schema.TimeoutDelete)
	_, err := waiter.Conf(timeout).WaitForStateContext(ctx)
	if err != nil {
		return diag.Errorf("error waiting for Aiven Database to be DELETED: %s", err)
	}

	return nil
}

func resourceDatabaseState(ctx context.Context, d *schema.ResourceData, m interface{}) ([]*schema.ResourceData, error) {
	if len(strings.Split(d.Id(), "/")) != 3 {
		return nil, fmt.Errorf("invalid identifier %v, expected <project_name>/<service_name>/<database_name>", d.Id())
	}

	di := resourceDatabaseRead(ctx, d, m)
	if di.HasError() {
		return nil, fmt.Errorf("cannot get database: %v", di)
	}

	return []*schema.ResourceData{d}, nil
}

// DatabaseDeleteWaiter is used to wait for Database to be deleted.
type DatabaseDeleteWaiter struct {
	Client      *aiven.Client
	ProjectName string
	ServiceName string
	Database    string
}

// RefreshFunc will call the Aiven client and refresh it's state.
func (w *DatabaseDeleteWaiter) RefreshFunc() resource.StateRefreshFunc {
	return func() (interface{}, string, error) {
		err := w.Client.Databases.Delete(w.ProjectName, w.ServiceName, w.Database)
		if err != nil && !aiven.IsNotFound(err) {
			return nil, "REMOVING", nil
		}

		return aiven.Database{}, "DELETED", nil
	}
}

// Conf sets up the configuration to refresh.
func (w *DatabaseDeleteWaiter) Conf(timeout time.Duration) *resource.StateChangeConf {
	return &resource.StateChangeConf{
		Pending:    []string{"REMOVING"},
		Target:     []string{"DELETED"},
		Refresh:    w.RefreshFunc(),
		Delay:      5 * time.Second,
		Timeout:    timeout,
		MinTimeout: 5 * time.Second,
	}
}
