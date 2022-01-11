// Copyright (c) 2017 jelmersnoeck
// Copyright (c) 2018-2021 Aiven, Helsinki, Finland. https://aiven.io/
package aiven

import (
	"context"

	"github.com/aiven/aiven-go-client"
	"github.com/aiven/terraform-provider-aiven/aiven/internal/schemautil"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func datasourceProjectUser() *schema.Resource {
	return &schema.Resource{
		ReadContext: datasourceProjectUserRead,
		Description: "The Project User data source provides information about the existing Aiven Project User.",
		Schema: resourceSchemaAsDatasourceSchema(aivenProjectUserSchema,
			"project", "email"),
	}
}

func datasourceProjectUserRead(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*aiven.Client)

	projectName := d.Get("project").(string)
	email := d.Get("email").(string)

	users, invitations, err := client.ProjectUsers.List(projectName)
	if err != nil {
		return diag.FromErr(err)
	}
	for _, user := range users {
		if user.Email == email {
			d.SetId(schemautil.BuildResourceID(projectName, email))
			return resourceProjectUserRead(ctx, d, m)
		}
	}

	for _, invitation := range invitations {
		if invitation.UserEmail == email {
			d.SetId(schemautil.BuildResourceID(projectName, email))
			return resourceProjectUserRead(ctx, d, m)
		}
	}

	return diag.Errorf("project user %s/%s not found", projectName, email)
}
