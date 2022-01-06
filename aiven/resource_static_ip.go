// Copyright (c) 2017 jelmersnoeck
// Copyright (c) 2018-2021 Aiven, Helsinki, Finland. https://aiven.io/
package aiven

import (
	"context"
	"fmt"

	"github.com/aiven/aiven-go-client"
	"github.com/aiven/terraform-provider-aiven/aiven/internal/schemautil"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

var aivenStaticIPSchema = map[string]*schema.Schema{
	"project": commonSchemaProjectReference,

	"cloud_name": {
		Type:        schema.TypeString,
		Required:    true,
		ForceNew:    true,
		Description: complex("Specifies the cloud that the static ip belongs to.").forceNew().build(),
	},
	"ip_address": {
		Type:        schema.TypeString,
		Computed:    true,
		Description: complex("The address of the static ip").build(),
	},
	"service_name": {
		Type:        schema.TypeString,
		Computed:    true,
		Description: complex("The service name the static ip is associated with").build(),
	},
	"state": {
		Type:        schema.TypeString,
		Computed:    true,
		Description: complex("The state the static ip is in").build(),
	},
	"static_ip_address_id": {
		Type:        schema.TypeString,
		Computed:    true,
		Description: "The Table ID of the flink table in the flink service.",
	},
}

func resourceStaticIP() *schema.Resource {
	return &schema.Resource{
		Description:   "The aiven static_ip resource allows the creation and deletion of static ips.",
		CreateContext: resourceStaticIPCreate,
		ReadContext:   resourceStaticIPRead,
		DeleteContext: resourceStaticIPDelete,
		Schema:        aivenStaticIPSchema,
	}
}

func resourceStaticIPRead(_ context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*aiven.Client)

	project, staticIPAddressId := schemautil.SplitResourceID2(d.Id())

	r, err := client.StaticIPs.List(project)
	if err != nil {
		return diag.FromErr(resourceReadHandleNotFound(err, d))
	}
	for i := range r.StaticIPs {
		sip := r.StaticIPs[i]
		if sip.StaticIPAddressID == staticIPAddressId {
			if err := d.Set("project", project); err != nil {
				return diag.Errorf("error setting static ips `project` for resource %s: %s", d.Id(), err)
			}
			if err := d.Set("cloud_name", sip.CloudName); err != nil {
				return diag.Errorf("error setting static ips `cloud_name` for resource %s: %s", d.Id(), err)
			}
			if err := d.Set("ip_address", sip.IPAddress); err != nil {
				return diag.Errorf("error setting static ips `ip_address` for resource %s: %s", d.Id(), err)
			}
			if err := d.Set("service_name", sip.ServiceName); err != nil {
				return diag.Errorf("error setting static ips `service_name` for resource %s: %s", d.Id(), err)
			}
			if err := d.Set("state", sip.State); err != nil {
				return diag.Errorf("error setting static ips `state` for resource %s: %s", d.Id(), err)
			}
			if err := d.Set("static_ip_address_id", sip.StaticIPAddressID); err != nil {
				return diag.Errorf("error setting static ips `static_ip_address_id` for resource %s: %s", d.Id(), err)
			}
			return nil
		}
	}
	d.SetId("")
	return nil
}

func resourceStaticIPCreate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*aiven.Client)

	project := d.Get("project").(string)
	cloudName := d.Get("cloud_name").(string)

	r, err := client.StaticIPs.Create(project, aiven.CreateStaticIPRequest{CloudName: cloudName})
	if err != nil {
		return diag.FromErr(fmt.Errorf("unable to create static ip: %w", err))
	}

	d.SetId(schemautil.BuildResourceID(project, r.StaticIPAddressID))

	if err := resourceStaticIPWait(ctx, d, m); err != nil {
		return diag.FromErr(fmt.Errorf("unable to wait for static ip to become active: %w", err))
	}

	return resourceStaticIPRead(ctx, d, m)
}

func resourceStaticIPDelete(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*aiven.Client)

	project, staticIPAddressId := schemautil.SplitResourceID2(d.Id())

	err := client.StaticIPs.Delete(
		project,
		aiven.DeleteStaticIPRequest{
			StaticIPAddressID: staticIPAddressId,
		})
	if err != nil && !aiven.IsNotFound(err) {
		return diag.FromErr(err)
	}
	return nil
}

func resourceStaticIPWait(ctx context.Context, d *schema.ResourceData, m interface{}) error {
	client := m.(*aiven.Client)

	project, staticIPAddressId := schemautil.SplitResourceID2(d.Id())

	conf := resource.StateChangeConf{
		Target:  []string{"created"},
		Pending: []string{"creating", "waiting"},
		Timeout: d.Timeout(schema.TimeoutCreate),
		Refresh: func() (result interface{}, state string, err error) {
			r, err := client.StaticIPs.List(project)
			if err != nil {
				return nil, "", fmt.Errorf("unable to fetch static ips: %w", err)
			}
			for i := range r.StaticIPs {
				sip := r.StaticIPs[i]

				if sip.StaticIPAddressID == staticIPAddressId {
					return struct{}{}, sip.State, nil
				}
			}
			return struct{}{}, "waiting", nil
		},
	}

	if _, err := conf.WaitForStateContext(ctx); err != nil {
		return fmt.Errorf("error waiting for static ip to be created: %s", err)
	}

	return nil
}
