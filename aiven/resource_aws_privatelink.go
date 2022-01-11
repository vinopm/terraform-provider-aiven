// Copyright (c) 2017 jelmersnoeck
// Copyright (c) 2018-2021 Aiven, Helsinki, Finland. https://aiven.io/
package aiven

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/aiven/aiven-go-client"
	"github.com/aiven/terraform-provider-aiven/aiven/internal/schemautil"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

var aivenAWSPrivatelinkSchema = map[string]*schema.Schema{
	"project":      commonSchemaProjectReference,
	"service_name": commonSchemaServiceNameReference,
	"principals": {
		Type:        schema.TypeSet,
		Required:    true,
		Description: "List of allowed principals",
		Elem:        &schema.Schema{Type: schema.TypeString},
	},
	"aws_service_id": {
		Type:        schema.TypeString,
		Computed:    true,
		Description: "AWS service ID",
	},
	"aws_service_name": {
		Type:        schema.TypeString,
		Computed:    true,
		Description: "AWS service name",
	},
}

func resourceAWSPrivatelink() *schema.Resource {
	return &schema.Resource{
		Description:   "The AWS Privatelink resource allows the creation and management of Aiven AWS Privatelink for a services.",
		CreateContext: resourceAWSPrivatelinkCreate,
		ReadContext:   resourceAWSPrivatelinkRead,
		UpdateContext: resourceAWSPrivatelinkUpdate,
		DeleteContext: resourceAWSPrivatelinkDelete,
		Importer: &schema.ResourceImporter{
			StateContext: resourceAWSPrivatelinkState,
		},
		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(20 * time.Minute),
			Update: schema.DefaultTimeout(20 * time.Minute),
		},

		Schema: aivenAWSPrivatelinkSchema,
	}
}

func resourceAWSPrivatelinkCreate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*aiven.Client)

	var principals []string
	var project = d.Get("project").(string)
	var serviceName = d.Get("service_name").(string)

	for _, p := range d.Get("principals").(*schema.Set).List() {
		principals = append(principals, p.(string))
	}

	_, err := client.AWSPrivatelink.Create(
		project,
		serviceName,
		principals,
	)
	if err != nil {
		return diag.FromErr(err)
	}

	// Wait until the AWS privatelink is active
	w := &AWSPrivatelinkWaiter{
		Client:      m.(*aiven.Client),
		Project:     project,
		ServiceName: serviceName,
	}

	_, err = w.Conf(d.Timeout(schema.TimeoutCreate)).WaitForStateContext(ctx)
	if err != nil {
		return diag.Errorf("Error waiting for AWS privatelink creation: %s", err)
	}

	d.SetId(schemautil.BuildResourceID(project, serviceName))

	return resourceAWSPrivatelinkRead(ctx, d, m)
}

func resourceAWSPrivatelinkRead(_ context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*aiven.Client)

	project, serviceName := schemautil.SplitResourceID2(d.Id())
	p, err := client.AWSPrivatelink.Get(project, serviceName)
	if err != nil {
		return diag.FromErr(resourceReadHandleNotFound(err, d))
	}

	if err := d.Set("principals", p.Principals); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("aws_service_id", p.AWSServiceID); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("aws_service_name", p.AWSServiceName); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("project", project); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("service_name", serviceName); err != nil {
		return diag.FromErr(err)
	}

	return nil
}
func resourceAWSPrivatelinkUpdate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*aiven.Client)

	project, serviceName := schemautil.SplitResourceID2(d.Id())

	var principals []string
	for _, p := range d.Get("principals").(*schema.Set).List() {
		principals = append(principals, p.(string))
	}

	_, err := client.AWSPrivatelink.Update(
		project,
		serviceName,
		principals,
	)
	if err != nil && !aiven.IsAlreadyExists(err) {
		return diag.FromErr(err)
	}

	// Wait until the AWS privatelink is active
	w := &AWSPrivatelinkWaiter{
		Client:      m.(*aiven.Client),
		Project:     project,
		ServiceName: serviceName,
	}

	_, err = w.Conf(d.Timeout(schema.TimeoutCreate)).WaitForStateContext(ctx)
	if err != nil {
		return diag.Errorf("Error waiting for AWS privatelink to be updated: %s", err)
	}

	return resourceAWSPrivatelinkRead(ctx, d, m)
}

func resourceAWSPrivatelinkDelete(_ context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*aiven.Client)

	err := client.AWSPrivatelink.Delete(schemautil.SplitResourceID2(d.Id()))
	if err != nil && !aiven.IsNotFound(err) {
		return diag.FromErr(err)
	}

	return nil
}

func resourceAWSPrivatelinkState(ctx context.Context, d *schema.ResourceData, m interface{}) ([]*schema.ResourceData, error) {
	di := resourceAWSPrivatelinkRead(ctx, d, m)
	if di.HasError() {
		return nil, fmt.Errorf("cannot get AWS privatelink %v", di)
	}

	return []*schema.ResourceData{d}, nil
}

// AWSPrivatelinkWaiter is used to wait for Aiven to build a AWS privatelink
type AWSPrivatelinkWaiter struct {
	Client      *aiven.Client
	Project     string
	ServiceName string
}

// RefreshFunc will call the Aiven client and refresh it's state.
func (w *AWSPrivatelinkWaiter) RefreshFunc() resource.StateRefreshFunc {
	return func() (interface{}, string, error) {
		pc, err := w.Client.AWSPrivatelink.Get(w.Project, w.ServiceName)
		if err != nil {
			return nil, "", err
		}

		log.Printf("[DEBUG] Got %s state while waiting for AWS privatelink to be active.", pc.State)

		return pc, pc.State, nil
	}
}

// Conf sets up the configuration to refresh.
func (w *AWSPrivatelinkWaiter) Conf(timeout time.Duration) *resource.StateChangeConf {
	log.Printf("[DEBUG] Create waiter timeout %.0f minutes", timeout.Minutes())

	return &resource.StateChangeConf{
		Pending: []string{"creating"},
		Target:  []string{"active"},
		Refresh: w.RefreshFunc(),
		Delay:   10 * time.Second,
		Timeout: timeout,
	}
}
