// Copyright (c) 2017 jelmersnoeck
// Copyright (c) 2018-2021 Aiven, Helsinki, Finland. https://aiven.io/
package aiven

import (
	"time"

	"github.com/aiven/terraform-provider-aiven/aiven/internal/service"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/customdiff"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func aivenKafkaConnectSchema() map[string]*schema.Schema {
	kafkaConnectSchema := serviceCommonSchema()
	kafkaConnectSchema[ServiceTypeKafkaConnect] = &schema.Schema{
		Type:        schema.TypeList,
		Computed:    true,
		Description: "Kafka Connect server provided values",
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{},
		},
	}
	kafkaConnectSchema[ServiceTypeKafkaConnect+"_user_config"] = service.GenerateServiceUserConfigurationSchema(ServiceTypeKafkaConnect)

	return kafkaConnectSchema
}

func resourceKafkaConnect() *schema.Resource {
	return &schema.Resource{
		Description:   "The Kafka Connect resource allows the creation and management of Aiven Kafka Connect services.",
		CreateContext: resourceServiceCreateWrapper(ServiceTypeKafkaConnect),
		ReadContext:   resourceServiceRead,
		UpdateContext: resourceServiceUpdate,
		DeleteContext: resourceServiceDelete,
		CustomizeDiff: customdiff.All(
			customdiff.Sequence(
				service.SetServiceTypeIfEmpty(ServiceTypeKafkaConnect),
				customdiff.IfValueChange("disk_space",
					service.DiskSpaceShouldNotBeEmpty,
					service.CustomizeDiffCheckDiskSpace),
			),
			customdiff.IfValueChange("service_integrations",
				service.ServiceIntegrationShouldNotBeEmpty,
				service.CustomizeDiffServiceIntegrationAfterCreation),
		),
		Importer: &schema.ResourceImporter{
			StateContext: resourceServiceState,
		},
		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(20 * time.Minute),
			Update: schema.DefaultTimeout(20 * time.Minute),
		},

		Schema: aivenKafkaConnectSchema(),
	}
}
