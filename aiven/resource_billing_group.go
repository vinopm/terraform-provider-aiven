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

var aivenBillingGroupSchema = map[string]*schema.Schema{
	"name": {
		Type:        schema.TypeString,
		Required:    true,
		Description: "Billing Group name",
	},
	"card_id": {
		Type:             schema.TypeString,
		Optional:         true,
		DiffSuppressFunc: schemautil.EmptyObjectNoChangeDiffSuppressFunc,
		Description:      "Credit card id",
	},
	"vat_id": {
		Type:             schema.TypeString,
		Optional:         true,
		DiffSuppressFunc: schemautil.EmptyObjectNoChangeDiffSuppressFunc,
		Description:      "VAT id",
	},
	"account_id": {
		Type:             schema.TypeString,
		Optional:         true,
		DiffSuppressFunc: schemautil.EmptyObjectNoChangeDiffSuppressFunc,
		Description:      "Account id",
	},
	"billing_currency": {
		Type:             schema.TypeString,
		Optional:         true,
		DiffSuppressFunc: schemautil.EmptyObjectNoChangeDiffSuppressFunc,
		Description:      "Billing currency",
	},
	"billing_extra_text": {
		Type:             schema.TypeString,
		Optional:         true,
		DiffSuppressFunc: schemautil.EmptyObjectNoChangeDiffSuppressFunc,
		Description:      "Billing extra text",
	},
	"billing_emails": {
		Type:             schema.TypeSet,
		Elem:             &schema.Schema{Type: schema.TypeString},
		Optional:         true,
		DiffSuppressFunc: schemautil.EmptyObjectNoChangeDiffSuppressFunc,
		Description:      "Billing contact emails",
	},
	"company": {
		Type:             schema.TypeString,
		Optional:         true,
		DiffSuppressFunc: schemautil.EmptyObjectNoChangeDiffSuppressFunc,
		Description:      "Company name",
	},
	"address_lines": {
		Type:             schema.TypeSet,
		Elem:             &schema.Schema{Type: schema.TypeString},
		Optional:         true,
		DiffSuppressFunc: schemautil.EmptyObjectNoChangeDiffSuppressFunc,
		Description:      "Address lines",
	},
	"country_code": {
		Type:             schema.TypeString,
		Optional:         true,
		DiffSuppressFunc: schemautil.EmptyObjectNoChangeDiffSuppressFunc,
		Description:      "Country code",
	},
	"city": {
		Type:             schema.TypeString,
		Optional:         true,
		DiffSuppressFunc: schemautil.EmptyObjectNoChangeDiffSuppressFunc,
		Description:      "City",
	},
	"zip_code": {
		Type:             schema.TypeString,
		Optional:         true,
		DiffSuppressFunc: schemautil.EmptyObjectNoChangeDiffSuppressFunc,
		Description:      "Zip Code",
	},
	"state": {
		Type:             schema.TypeString,
		Optional:         true,
		DiffSuppressFunc: schemautil.EmptyObjectNoChangeDiffSuppressFunc,
		Description:      "State",
	},
}

func resourceBillingGroup() *schema.Resource {
	return &schema.Resource{
		Description:   "The Billing Group resource allows the creation and management of Aiven Billing Groups and association with the Projects.",
		CreateContext: resourceBillingGroupCreate,
		ReadContext:   resourceBillingGroupRead,
		UpdateContext: resourceBillingGroupUpdate,
		DeleteContext: resourceBillingGroupDelete,
		Importer: &schema.ResourceImporter{
			StateContext: resourceBillingGroupState,
		},

		Schema: aivenBillingGroupSchema,
	}
}

func resourceBillingGroupCreate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*aiven.Client)

	var billingEmails []*aiven.ContactEmail
	if emails := contactEmailListForAPI(d, "billing_emails", true); emails != nil {
		billingEmails = *emails
	}

	cardID, err := getLongCardID(client, d.Get("card_id").(string))
	if err != nil {
		return diag.Errorf("Error getting long card id: %s", err)
	}

	bg, err := client.BillingGroup.Create(
		aiven.BillingGroupRequest{
			BillingGroupName: d.Get("name").(string),
			AccountId:        schemautil.OptionalStringPointer(d, "account_id"),
			CardId:           cardID,
			VatId:            schemautil.OptionalStringPointer(d, "vat_id"),
			BillingCurrency:  schemautil.OptionalStringPointer(d, "billing_currency"),
			BillingExtraText: schemautil.OptionalStringPointer(d, "billing_extra_text"),
			BillingEmails:    billingEmails,
			Company:          schemautil.OptionalStringPointer(d, "company"),
			AddressLines:     schemautil.FlattenToString(d.Get("address_lines").(*schema.Set).List()),
			CountryCode:      schemautil.OptionalStringPointer(d, "country_code"),
			City:             schemautil.OptionalStringPointer(d, "city"),
			ZipCode:          schemautil.OptionalStringPointer(d, "zip_code"),
			State:            schemautil.OptionalStringPointer(d, "state"),
		},
	)
	if err != nil {
		return diag.Errorf("cannot create billing group: %s", err)
	}

	d.SetId(bg.Id)

	return resourceBillingGroupRead(ctx, d, m)
}

func resourceBillingGroupRead(_ context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*aiven.Client)

	bg, err := client.BillingGroup.Get(d.Id())
	if err != nil {
		return diag.FromErr(resourceReadHandleNotFound(err, d))
	}

	if err := d.Set("name", bg.BillingGroupName); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("account_id", bg.AccountId); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("card_id", bg.CardId); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("vat_id", bg.VatId); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("billing_currency", bg.BillingCurrency); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("billing_extra_text", bg.BillingExtraText); err != nil {
		return diag.FromErr(err)
	}
	if err := contactEmailListForTerraform(d, "billing_emails", bg.BillingEmails); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("company", bg.Company); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("address_lines", bg.AddressLines); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("country_code", bg.CountryCode); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("city", bg.City); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("zip_code", bg.ZipCode); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("state", bg.State); err != nil {
		return diag.FromErr(err)
	}

	return nil
}

func resourceBillingGroupUpdate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*aiven.Client)

	var billingEmails []*aiven.ContactEmail
	if emails := contactEmailListForAPI(d, "billing_emails", true); emails != nil {
		billingEmails = *emails
	}

	cardID, err := getLongCardID(client, d.Get("card_id").(string))
	if err != nil {
		return diag.Errorf("Error getting long card id: %s", err)
	}

	bg, err := client.BillingGroup.Update(
		d.Id(),
		aiven.BillingGroupRequest{
			BillingGroupName: d.Get("name").(string),
			AccountId:        schemautil.OptionalStringPointer(d, "account_id"),
			CardId:           cardID,
			VatId:            schemautil.OptionalStringPointer(d, "vat_id"),
			BillingCurrency:  schemautil.OptionalStringPointer(d, "billing_currency"),
			BillingExtraText: schemautil.OptionalStringPointer(d, "billing_extra_text"),
			BillingEmails:    billingEmails,
			Company:          schemautil.OptionalStringPointer(d, "company"),
			AddressLines:     schemautil.FlattenToString(d.Get("address_lines").(*schema.Set).List()),
			CountryCode:      schemautil.OptionalStringPointer(d, "country_code"),
			City:             schemautil.OptionalStringPointer(d, "city"),
			ZipCode:          schemautil.OptionalStringPointer(d, "zip_code"),
			State:            schemautil.OptionalStringPointer(d, "state"),
		},
	)
	if err != nil {
		return diag.Errorf("cannot update billing group: %s", err)
	}

	d.SetId(bg.Id)

	return resourceBillingGroupRead(ctx, d, m)
}

func resourceBillingGroupDelete(_ context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*aiven.Client)

	err := client.BillingGroup.Delete(d.Id())
	if err != nil && !aiven.IsNotFound(err) {
		return diag.Errorf("cannot delete a billing group: %s", err)
	}

	return nil
}

func resourceBillingGroupState(ctx context.Context, d *schema.ResourceData, m interface{}) ([]*schema.ResourceData, error) {
	di := resourceBillingGroupRead(ctx, d, m)
	if di.HasError() {
		return nil, fmt.Errorf("cannot get a billing group: %v", di)
	}

	return []*schema.ResourceData{d}, nil
}
