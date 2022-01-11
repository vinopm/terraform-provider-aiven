// Copyright (c) 2017 jelmersnoeck
// Copyright (c) 2018-2021 Aiven, Helsinki, Finland. https://aiven.io/
package aiven

import (
	"time"

	"github.com/aiven/terraform-provider-aiven/aiven/internal/service"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func cassandraSchema() map[string]*schema.Schema {
	s := serviceCommonSchema()
	s[ServiceTypeCassandra] = &schema.Schema{
		Type:        schema.TypeList,
		Computed:    true,
		Description: "Cassandra server provided values",
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{},
		},
	}
	s[ServiceTypeCassandra+"_user_config"] = service.GenerateServiceUserConfigurationSchema(ServiceTypeCassandra)

	return s
}

func resourceCassandra() *schema.Resource {
	return &schema.Resource{
		Description:   "The Cassandra resource allows the creation and management of Aiven Cassandra services.",
		CreateContext: resourceServiceCreateWrapper(ServiceTypeCassandra),
		ReadContext:   resourceServiceRead,
		UpdateContext: resourceServiceUpdate,
		DeleteContext: resourceServiceDelete,
		Importer: &schema.ResourceImporter{
			StateContext: resourceServiceState,
		},
		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(20 * time.Minute),
			Update: schema.DefaultTimeout(20 * time.Minute),
		},

		Schema: cassandraSchema(),
	}
}
