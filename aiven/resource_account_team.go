// Copyright (c) 2017 jelmersnoeck
// Copyright (c) 2018-2021 Aiven, Helsinki, Finland. https://aiven.io/
package aiven

import (
	"context"
	"fmt"

	"github.com/aiven/aiven-go-client"
	"github.com/aiven/terraform-provider-aiven/aiven/internal/schemautil"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

var aivenAccountTeamSchema = map[string]*schema.Schema{
	"account_id": {
		Type:        schema.TypeString,
		Required:    true,
		Description: "The unique account id",
	},
	"team_id": {
		Type:        schema.TypeString,
		Computed:    true,
		Description: "The auto-generated unique account team id",
	},
	"name": {
		Type:        schema.TypeString,
		Required:    true,
		Description: "The account team name",
	},
	"create_time": {
		Type:        schema.TypeString,
		Computed:    true,
		Description: "Time of creation",
	},
	"update_time": {
		Type:        schema.TypeString,
		Computed:    true,
		Description: "Time of last update",
	},
}

func resourceAccountTeam() *schema.Resource {
	return &schema.Resource{
		Description:   "The Account Team resource allows the creation and management of an Account Team.",
		CreateContext: resourceAccountTeamCreate,
		ReadContext:   resourceAccountTeamRead,
		UpdateContext: resourceAccountTeamUpdate,
		DeleteContext: resourceAccountTeamDelete,
		Importer: &schema.ResourceImporter{
			StateContext: resourceAccountTeamState,
		},

		Schema: aivenAccountTeamSchema,
	}
}

func resourceAccountTeamCreate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*aiven.Client)
	name := d.Get("name").(string)
	accountId := d.Get("account_id").(string)

	r, err := client.AccountTeams.Create(
		accountId,
		aiven.AccountTeam{
			Name: name,
		},
	)
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(schemautil.BuildResourceID(r.Team.AccountId, r.Team.Id))

	return resourceAccountTeamRead(ctx, d, m)
}

func resourceAccountTeamRead(_ context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*aiven.Client)

	accountId, teamId := schemautil.SplitResourceID2(d.Id())
	r, err := client.AccountTeams.Get(accountId, teamId)
	if err != nil {
		return diag.FromErr(resourceReadHandleNotFound(err, d))
	}

	if err := d.Set("account_id", r.Team.AccountId); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("team_id", r.Team.Id); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("name", r.Team.Name); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("create_time", r.Team.CreateTime.String()); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("update_time", r.Team.UpdateTime.String()); err != nil {
		return diag.FromErr(err)
	}

	return nil
}

func resourceAccountTeamUpdate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*aiven.Client)
	accountId, teamId := schemautil.SplitResourceID2(d.Id())

	r, err := client.AccountTeams.Update(accountId, teamId, aiven.AccountTeam{
		Name: d.Get("name").(string),
	})
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(schemautil.BuildResourceID(r.Team.AccountId, r.Team.Id))

	return resourceAccountTeamRead(ctx, d, m)
}

func resourceAccountTeamDelete(_ context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*aiven.Client)

	accountId, teamId := schemautil.SplitResourceID2(d.Id())

	err := client.AccountTeams.Delete(accountId, teamId)
	if err != nil && !aiven.IsNotFound(err) {
		return diag.FromErr(err)
	}

	return nil
}

func resourceAccountTeamState(ctx context.Context, d *schema.ResourceData, m interface{}) ([]*schema.ResourceData, error) {
	di := resourceAccountTeamRead(ctx, d, m)
	if di.HasError() {
		return nil, fmt.Errorf("cannot get account team %v", di)
	}

	return []*schema.ResourceData{d}, nil
}
