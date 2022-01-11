// Copyright (c) 2017 jelmersnoeck
// Copyright (c) 2018-2021 Aiven, Helsinki, Finland. https://aiven.io/
package aiven

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/aiven/aiven-go-client"
	"github.com/aiven/terraform-provider-aiven/aiven/internal/schemautil"
	"github.com/aiven/terraform-provider-aiven/aiven/internal/service"
	"github.com/aiven/terraform-provider-aiven/aiven/internal/uconf"

	"github.com/docker/go-units"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/customdiff"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

const (
	ServiceTypePG               = "pg"
	ServiceTypeCassandra        = "cassandra"
	ServiceTypeElasticsearch    = "elasticsearch"
	ServiceTypeOpensearch       = "opensearch"
	ServiceTypeGrafana          = "grafana"
	ServiceTypeInfluxDB         = "influxdb"
	ServiceTypeRedis            = "redis"
	ServiceTypeMySQL            = "mysql"
	ServiceTypeKafka            = "kafka"
	ServiceTypeKafkaConnect     = "kafka_connect"
	ServiceTypeKafkaMirrormaker = "kafka_mirrormaker"
	ServiceTypeM3               = "m3db"
	ServiceTypeM3Aggregator     = "m3aggregator"
	ServiceTypeFlink            = "flink"
	ServiceTypeClickhouse       = "clickhouse"
)

func availableServiceTypes() []string {
	return []string{
		ServiceTypePG,
		ServiceTypeCassandra,
		ServiceTypeElasticsearch,
		ServiceTypeGrafana,
		ServiceTypeInfluxDB,
		ServiceTypeRedis,
		ServiceTypeMySQL,
		ServiceTypeKafka,
		ServiceTypeKafkaConnect,
		ServiceTypeKafkaMirrormaker,
		ServiceTypeM3,
		ServiceTypeM3Aggregator,
		ServiceTypeOpensearch,
		ServiceTypeFlink,
		ServiceTypeClickhouse,
	}
}

func serviceCommonSchema() map[string]*schema.Schema {
	return map[string]*schema.Schema{
		"project": commonSchemaProjectReference,

		"cloud_name": {
			Type:        schema.TypeString,
			Optional:    true,
			Description: "Defines where the cloud provider and region where the service is hosted in. This can be changed freely after service is created. Changing the value will trigger a potentially lengthy migration process for the service. Format is cloud provider name (`aws`, `azure`, `do` `google`, `upcloud`, etc.), dash, and the cloud provider specific region name. These are documented on each Cloud provider's own support articles, like [here for Google](https://cloud.google.com/compute/docs/regions-zones/) and [here for AWS](https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/Concepts.RegionsAndAvailabilityZones.html).",
		},
		"plan": {
			Type:        schema.TypeString,
			Optional:    true,
			Description: "Defines what kind of computing resources are allocated for the service. It can be changed after creation, though there are some restrictions when going to a smaller plan such as the new plan must have sufficient amount of disk space to store all current data and switching to a plan with fewer nodes might not be supported. The basic plan names are `hobbyist`, `startup-x`, `business-x` and `premium-x` where `x` is (roughly) the amount of memory on each node (also other attributes like number of CPUs and amount of disk space varies but naming is based on memory). The available options can be seem from the [Aiven pricing page](https://aiven.io/pricing).",
		},
		"service_name": {
			Type:        schema.TypeString,
			Required:    true,
			ForceNew:    true,
			Description: "Specifies the actual name of the service. The name cannot be changed later without destroying and re-creating the service so name should be picked based on intended service usage rather than current attributes.",
		},
		"service_type": {
			Type:        schema.TypeString,
			Computed:    true,
			Description: "Aiven internal service type code",
		},
		"project_vpc_id": {
			Type:        schema.TypeString,
			Optional:    true,
			Description: "Specifies the VPC the service should run in. If the value is not set the service is not run inside a VPC. When set, the value should be given as a reference to set up dependencies correctly and the VPC must be in the same cloud and region as the service itself. Project can be freely moved to and from VPC after creation but doing so triggers migration to new servers so the operation can take significant amount of time to complete if the service has a lot of data.",
		},
		"maintenance_window_dow": {
			Type:        schema.TypeString,
			Optional:    true,
			Description: "Day of week when maintenance operations should be performed. One monday, tuesday, wednesday, etc.",
			DiffSuppressFunc: func(k, old, new string, d *schema.ResourceData) bool {
				return new == ""
			},
		},
		"maintenance_window_time": {
			Type:        schema.TypeString,
			Optional:    true,
			Description: "Time of day when maintenance operations should be performed. UTC time in HH:mm:ss format.",
			DiffSuppressFunc: func(k, old, new string, d *schema.ResourceData) bool {
				return new == ""
			},
		},
		"termination_protection": {
			Type:        schema.TypeBool,
			Optional:    true,
			Description: "Prevents the service from being deleted. It is recommended to set this to `true` for all production services to prevent unintentional service deletion. This does not shield against deleting databases or topics but for services with backups much of the content can at least be restored from backup in case accidental deletion is done.",
		},
		"disk_space": {
			Type:         schema.TypeString,
			Optional:     true,
			Description:  "The disk space of the service, possible values depend on the service type, the cloud provider and the project. Reducing will result in the service rebalancing.",
			ValidateFunc: schemautil.ValidateHumanByteSizeString,
		},
		"disk_space_used": {
			Type:        schema.TypeString,
			Computed:    true,
			Description: "Disk space that service is currently using",
		},
		"disk_space_default": {
			Type:        schema.TypeString,
			Computed:    true,
			Description: "The default disk space of the service, possible values depend on the service type, the cloud provider and the project. Its also the minimum value for `disk_space`",
		},
		"disk_space_step": {
			Type:        schema.TypeString,
			Computed:    true,
			Description: "The default disk space step of the service, possible values depend on the service type, the cloud provider and the project. `disk_space` needs to increment from `disk_space_default` by increments of this size.",
		},
		"disk_space_cap": {
			Type:        schema.TypeString,
			Computed:    true,
			Description: "The maximum disk space of the service, possible values depend on the service type, the cloud provider and the project.",
		},
		"service_uri": {
			Type:        schema.TypeString,
			Computed:    true,
			Description: "URI for connecting to the service. Service specific info is under \"kafka\", \"pg\", etc.",
			Sensitive:   true,
		},
		"service_host": {
			Type:        schema.TypeString,
			Computed:    true,
			Description: "The hostname of the service.",
		},
		"service_port": {
			Type:        schema.TypeInt,
			Computed:    true,
			Description: "The port of the service",
		},
		"service_password": {
			Type:        schema.TypeString,
			Computed:    true,
			Description: "Password used for connecting to the service, if applicable",
			Sensitive:   true,
		},
		"service_username": {
			Type:        schema.TypeString,
			Computed:    true,
			Description: "Username used for connecting to the service, if applicable",
		},
		"state": {
			Type:        schema.TypeString,
			Computed:    true,
			Description: "Service state. One of `POWEROFF`, `REBALANCING`, `REBUILDING` or `RUNNING`.",
		},
		"service_integrations": {
			Type:        schema.TypeList,
			Optional:    true,
			Description: "Service integrations to specify when creating a service. Not applied after initial service creation",
			Elem: &schema.Resource{
				Schema: map[string]*schema.Schema{
					"source_service_name": {
						Type:        schema.TypeString,
						Required:    true,
						Description: "Name of the source service",
					},
					"integration_type": {
						Type:        schema.TypeString,
						Required:    true,
						Description: "Type of the service integration. The only supported value at the moment is `read_replica`",
					},
				},
			},
		},
		"components": {
			Type:        schema.TypeList,
			Computed:    true,
			Description: "Service component information objects",
			Elem: &schema.Resource{
				Schema: map[string]*schema.Schema{
					"component": {
						Type:        schema.TypeString,
						Computed:    true,
						Description: "Service component name",
					},
					"host": {
						Type:        schema.TypeString,
						Computed:    true,
						Description: "DNS name for connecting to the service component",
					},
					"kafka_authentication_method": {
						Type:        schema.TypeString,
						Computed:    true,
						Optional:    true,
						Description: "Kafka authentication method. This is a value specific to the 'kafka' service component",
					},
					"port": {
						Type:        schema.TypeInt,
						Computed:    true,
						Description: "Port number for connecting to the service component",
					},
					"route": {
						Type:        schema.TypeString,
						Computed:    true,
						Description: "Network access route",
					},
					"ssl": {
						Type:     schema.TypeBool,
						Computed: true,
						Description: "Whether the endpoint is encrypted or accepts plaintext. By default endpoints are " +
							"always encrypted and this property is only included for service components they may " +
							"disable encryption",
					},
					"usage": {
						Type:        schema.TypeString,
						Computed:    true,
						Description: "DNS usage name",
					},
				},
			},
		},
	}
}

var aivenServiceSchema = map[string]*schema.Schema{
	"project": {
		Type:        schema.TypeString,
		Required:    true,
		Description: "Target project",
		ForceNew:    true,
	},
	"cloud_name": {
		Type:        schema.TypeString,
		Optional:    true,
		Description: "Cloud the service runs in",
	},
	"plan": {
		Type:        schema.TypeString,
		Optional:    true,
		Description: "Subscription plan",
	},
	"service_name": {
		Type:        schema.TypeString,
		Required:    true,
		Description: "Service name",
		ForceNew:    true,
	},
	"service_type": {
		Type:         schema.TypeString,
		Required:     true,
		Description:  "Service type code",
		ForceNew:     true,
		ValidateFunc: validation.StringInSlice(availableServiceTypes(), false),
	},
	"project_vpc_id": {
		Type:        schema.TypeString,
		Optional:    true,
		Description: "Identifier of the VPC the service should be in, if any",
	},
	"maintenance_window_dow": {
		Type:        schema.TypeString,
		Optional:    true,
		Description: "Day of week when maintenance operations should be performed. One monday, tuesday, wednesday, etc.",
		DiffSuppressFunc: func(k, old, new string, d *schema.ResourceData) bool {
			return new == ""
		},
	},
	"maintenance_window_time": {
		Type:        schema.TypeString,
		Optional:    true,
		Description: "Time of day when maintenance operations should be performed. UTC time in HH:mm:ss format.",
		DiffSuppressFunc: func(k, old, new string, d *schema.ResourceData) bool {
			return new == ""
		},
	},
	"termination_protection": {
		Type:        schema.TypeBool,
		Optional:    true,
		Description: "Prevent service from being deleted. It is recommended to have this enabled for all services.",
	},
	"disk_space": {
		Type:             schema.TypeString,
		Optional:         true,
		Description:      "The disk space of the service, possible values depend on the service type, the cloud provider and the project. Reducing will result in the service rebalancing.",
		DiffSuppressFunc: schemautil.EmptyObjectDiffSuppressFunc,
		ValidateFunc:     schemautil.ValidateHumanByteSizeString,
	},
	"disk_space_default": {
		Type:        schema.TypeString,
		Computed:    true,
		Description: "The default disk space of the service, possible values depend on the service type, the cloud provider and the project. Its also the minimum value for `disk_space`",
	},
	"disk_space_step": {
		Type:        schema.TypeString,
		Computed:    true,
		Description: "The default disk space step of the service, possible values depend on the service type, the cloud provider and the project. `disk_space` needs to increment from `disk_space_default` by increments of this size.",
	},
	"disk_space_cap": {
		Type:        schema.TypeString,
		Computed:    true,
		Description: "The maximum disk space of the service, possible values depend on the service type, the cloud provider and the project.",
	},
	"service_uri": {
		Type:        schema.TypeString,
		Computed:    true,
		Description: "URI for connecting to the service. Service specific info is under \"kafka\", \"pg\", etc.",
		Sensitive:   true,
	},
	"service_host": {
		Type:        schema.TypeString,
		Computed:    true,
		Description: "Service hostname",
	},
	"service_integrations": {
		Type:        schema.TypeList,
		Optional:    true,
		Description: "Service integrations to specify when creating a service. Not applied after initial service creation",
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				"source_service_name": {
					Type:        schema.TypeString,
					Required:    true,
					Description: "Name of the source service",
				},
				"integration_type": {
					Type:        schema.TypeString,
					Required:    true,
					Description: "Type of the service integration. The only supported value at the moment is 'read_replica'",
				},
			},
		},
	},
	"components": {
		Type:        schema.TypeList,
		Computed:    true,
		Description: "Service component information objects",
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				"component": {
					Type:        schema.TypeString,
					Computed:    true,
					Description: "Service component name",
				},
				"host": {
					Type:        schema.TypeString,
					Computed:    true,
					Description: "DNS name for connecting to the service component",
				},
				"kafka_authentication_method": {
					Type:        schema.TypeString,
					Computed:    true,
					Optional:    true,
					Description: "Kafka authentication method. This is a value specific to the 'kafka' service component",
				},
				"port": {
					Type:        schema.TypeInt,
					Computed:    true,
					Description: "Port number for connecting to the service component",
				},
				"route": {
					Type:        schema.TypeString,
					Computed:    true,
					Description: "Network access route",
				},
				"ssl": {
					Type:     schema.TypeBool,
					Computed: true,
					Description: "Whether the endpoint is encrypted or accepts plaintext. By default endpoints are " +
						"always encrypted and this property is only included for service components they may " +
						"disable encryption",
				},
				"usage": {
					Type:        schema.TypeString,
					Computed:    true,
					Description: "DNS usage name",
				},
			},
		},
	},

	"service_port": {
		Type:        schema.TypeInt,
		Computed:    true,
		Description: "Service port",
	},
	"service_password": {
		Type:        schema.TypeString,
		Computed:    true,
		Description: "Password used for connecting to the service, if applicable",
		Sensitive:   true,
	},
	"service_username": {
		Type:        schema.TypeString,
		Computed:    true,
		Description: "Username used for connecting to the service, if applicable",
	},
	"state": {
		Type:        schema.TypeString,
		Computed:    true,
		Description: "Service state. One of `POWEROFF`, `REBALANCING`, `REBUILDING` and `RUNNING`.",
	},
	"cassandra": {
		Type:        schema.TypeList,
		Computed:    true,
		Description: "Cassandra specific server provided values",
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{},
		},
	},
	"cassandra_user_config": service.GenerateServiceUserConfigurationSchema(ServiceTypeCassandra),
	"elasticsearch": {
		Type:        schema.TypeList,
		Computed:    true,
		Description: "Elasticsearch specific server provided values",
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				"kibana_uri": {
					Type:        schema.TypeString,
					Computed:    true,
					Description: "URI for Kibana frontend",
					Sensitive:   true,
				},
			},
		},
	},
	"elasticsearch_user_config": service.GenerateServiceUserConfigurationSchema(ServiceTypeElasticsearch),
	"opensearch": {
		Type:        schema.TypeList,
		Computed:    true,
		Description: "Opensearch specific server provided values",
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				"opensearch_dashboards_uri": {
					Type:        schema.TypeString,
					Computed:    true,
					Description: "URI for Opensearch dashboard frontend",
					Sensitive:   true,
				},
			},
		},
	},
	"opensearch_user_config": service.GenerateServiceUserConfigurationSchema(ServiceTypeOpensearch),
	"grafana": {
		Type:        schema.TypeList,
		Computed:    true,
		Description: "Grafana specific server provided values",
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{},
		},
	},
	"grafana_user_config": service.GenerateServiceUserConfigurationSchema(ServiceTypeGrafana),
	"influxdb": {
		Type:        schema.TypeList,
		Computed:    true,
		Description: "InfluxDB specific server provided values",
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				"database_name": {
					Type:        schema.TypeString,
					Computed:    true,
					Description: "Name of the default InfluxDB database",
				},
			},
		},
	},
	"influxdb_user_config": service.GenerateServiceUserConfigurationSchema(ServiceTypeInfluxDB),
	"kafka": {
		Type:        schema.TypeList,
		Computed:    true,
		Description: "Kafka specific server provided values",
		Optional:    true,
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				"access_cert": {
					Type:        schema.TypeString,
					Computed:    true,
					Description: "The Kafka client certificate",
					Optional:    true,
					Sensitive:   true,
				},
				"access_key": {
					Type:        schema.TypeString,
					Computed:    true,
					Description: "The Kafka client certificate key",
					Optional:    true,
					Sensitive:   true,
				},
				"connect_uri": {
					Type:        schema.TypeString,
					Computed:    true,
					Description: "The Kafka Connect URI, if any",
					Optional:    true,
					Sensitive:   true,
				},
				"rest_uri": {
					Type:        schema.TypeString,
					Computed:    true,
					Description: "The Kafka REST URI, if any",
					Optional:    true,
					Sensitive:   true,
				},
				"schema_registry_uri": {
					Type:        schema.TypeString,
					Computed:    true,
					Description: "The Schema Registry URI, if any",
					Optional:    true,
					Sensitive:   true,
				},
			},
		},
	},
	"kafka_user_config": service.GenerateServiceUserConfigurationSchema(ServiceTypeKafka),
	"kafka_connect": {
		Type:        schema.TypeList,
		Computed:    true,
		Description: "Kafka Connect specific server provided values",
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{},
		},
	},
	"kafka_connect_user_config": service.GenerateServiceUserConfigurationSchema(ServiceTypeKafkaConnect),
	"mysql": {
		Type:        schema.TypeList,
		Computed:    true,
		Description: "MySQL specific server provided values",
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{},
		},
	},
	"mysql_user_config": service.GenerateServiceUserConfigurationSchema(ServiceTypeMySQL),
	"kafka_mirrormaker": {
		Type:        schema.TypeList,
		Computed:    true,
		Description: "Kafka MirrorMaker 2 specific server provided values",
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{},
		},
	},
	"kafka_mirrormaker_user_config": service.GenerateServiceUserConfigurationSchema(ServiceTypeKafkaMirrormaker),
	"pg": {
		Type:        schema.TypeList,
		Computed:    true,
		Description: "PostgreSQL specific server provided values",
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				"replica_uri": {
					Type:        schema.TypeString,
					Computed:    true,
					Description: "PostgreSQL replica URI for services with a replica",
					Sensitive:   true,
				},
				"uri": {
					Type:        schema.TypeString,
					Computed:    true,
					Description: "PostgreSQL master connection URI",
					Optional:    true,
					Sensitive:   true,
				},
				"dbname": {
					Type:        schema.TypeString,
					Computed:    true,
					Description: "Primary PostgreSQL database name",
				},
				"host": {
					Type:        schema.TypeString,
					Computed:    true,
					Description: "PostgreSQL master node host IP or name",
				},
				"password": {
					Type:        schema.TypeString,
					Computed:    true,
					Description: "PostgreSQL admin user password",
					Sensitive:   true,
				},
				"port": {
					Type:        schema.TypeInt,
					Computed:    true,
					Description: "PostgreSQL port",
				},
				"sslmode": {
					Type:        schema.TypeString,
					Computed:    true,
					Description: "PostgreSQL sslmode setting (currently always \"require\")",
				},
				"user": {
					Type:        schema.TypeString,
					Computed:    true,
					Description: "PostgreSQL admin user name",
				},
			},
		},
	},
	"pg_user_config": service.GenerateServiceUserConfigurationSchema(ServiceTypePG),
	"redis": {
		Type:        schema.TypeList,
		Computed:    true,
		Description: "Redis specific server provided values",
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{},
		},
	},
	"redis_user_config": service.GenerateServiceUserConfigurationSchema(ServiceTypeRedis),
	"clickhouse": {
		Type:        schema.TypeList,
		Computed:    true,
		Description: "Clickhouse specific server provided values",
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{},
		},
	},
	"clickhouse_user_config": service.GenerateServiceUserConfigurationSchema(ServiceTypeClickhouse),
	"flink": {
		Type:        schema.TypeList,
		Computed:    true,
		Description: "Flink specific server provided values",
		Optional:    true,
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				"host_ports": {
					Type:        schema.TypeList,
					Computed:    true,
					Description: "Host and Port of a Flink server",
					Optional:    true,
					Elem: &schema.Schema{
						Type: schema.TypeString,
					},
				},
			},
		},
	},
	"flink_user_config": service.GenerateServiceUserConfigurationSchema(ServiceTypeFlink),
}

func resourceService() *schema.Resource {
	return &schema.Resource{
		Description:        "The Service resource allows the creation and management of Aiven Services.",
		DeprecationMessage: "Please use the specific service resources instead of this resource.",
		CreateContext:      resourceServiceCreateWrapper("service"),
		ReadContext:        resourceServiceRead,
		UpdateContext:      resourceServiceUpdate,
		DeleteContext:      resourceServiceDelete,
		CustomizeDiff: customdiff.All(
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
		Schema: aivenServiceSchema,
	}
}

func resourceServiceCreateWrapper(serviceType string) schema.CreateContextFunc {
	if serviceType == "service" {
		return func(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
			// Need to set empty value for all services or all Terraform keeps on showing there's
			// a change in the computed values that don't match actual service type
			if err := d.Set(ServiceTypeCassandra, []map[string]interface{}{}); err != nil {
				return diag.FromErr(err)
			}
			if err := d.Set(ServiceTypeElasticsearch, []map[string]interface{}{}); err != nil {
				return diag.FromErr(err)
			}
			if err := d.Set(ServiceTypeGrafana, []map[string]interface{}{}); err != nil {
				return diag.FromErr(err)
			}
			if err := d.Set(ServiceTypeInfluxDB, []map[string]interface{}{}); err != nil {
				return diag.FromErr(err)
			}
			if err := d.Set(ServiceTypeKafka, []map[string]interface{}{}); err != nil {
				return diag.FromErr(err)
			}
			if err := d.Set(ServiceTypeKafkaConnect, []map[string]interface{}{}); err != nil {
				return diag.FromErr(err)
			}
			if err := d.Set(ServiceTypeKafkaMirrormaker, []map[string]interface{}{}); err != nil {
				return diag.FromErr(err)
			}
			if err := d.Set(ServiceTypeMySQL, []map[string]interface{}{}); err != nil {
				return diag.FromErr(err)
			}
			if err := d.Set(ServiceTypePG, []map[string]interface{}{}); err != nil {
				return diag.FromErr(err)
			}
			if err := d.Set(ServiceTypeRedis, []map[string]interface{}{}); err != nil {
				return diag.FromErr(err)
			}
			if err := d.Set(ServiceTypeOpensearch, []map[string]interface{}{}); err != nil {
				return diag.FromErr(err)
			}
			if err := d.Set(ServiceTypeFlink, []map[string]interface{}{}); err != nil {
				return diag.FromErr(err)
			}
			if err := d.Set(ServiceTypeClickhouse, []map[string]interface{}{}); err != nil {
				return diag.FromErr(err)
			}
			return resourceServiceCreate(ctx, d, m)
		}
	}

	return func(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
		if err := d.Set("service_type", serviceType); err != nil {
			return diag.FromErr(err)
		}
		if err := d.Set(serviceType, []map[string]interface{}{}); err != nil {
			return diag.FromErr(err)
		}

		return resourceServiceCreate(ctx, d, m)
	}
}

func resourceServiceRead(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*aiven.Client)

	projectName, serviceName := schemautil.SplitResourceID2(d.Id())
	s, err := client.Services.Get(projectName, serviceName)
	if err != nil {
		if err = resourceReadHandleNotFound(err, d); err != nil {
			return diag.FromErr(fmt.Errorf("unable to GET service %s: %s", d.Id(), err))
		}
		return nil
	}
	servicePlanParams, err := service.GetServicePlanParametersFromServiceResponse(ctx, client, projectName, s)
	if err != nil {
		return diag.FromErr(fmt.Errorf("unable to get service plan parameters: %w", err))
	}

	err = copyServicePropertiesFromAPIResponseToTerraform(d, s, servicePlanParams, projectName)
	if err != nil {
		return diag.FromErr(err)
	}

	return nil
}

func resourceServiceCreate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*aiven.Client)

	serviceType := d.Get("service_type").(string)
	project := d.Get("project").(string)

	// During the creation of service, if disc_space is not set by a TF user,
	// we transfer 0 values in API creation request, which makes Aiven provision
	// a default disc space values for a service
	var diskSpace int
	if ds, ok := d.GetOk("disk_space"); ok {
		diskSpace = service.ConvertToDiskSpaceMB(ds.(string))
	}

	_, err := client.Services.Create(
		project,
		aiven.CreateServiceRequest{
			Cloud:                 d.Get("cloud_name").(string),
			Plan:                  d.Get("plan").(string),
			ProjectVPCID:          service.GetProjectVPCIdPointer(d),
			ServiceIntegrations:   service.GetAPIServiceIntegrations(d),
			MaintenanceWindow:     service.GetMaintenanceWindow(d),
			ServiceName:           d.Get("service_name").(string),
			ServiceType:           serviceType,
			TerminationProtection: d.Get("termination_protection").(bool),
			DiskSpaceMB:           diskSpace,
			UserConfig:            uconf.ConvertTerraformUserConfigToAPICompatibleFormat("service", serviceType, true, d),
		},
	)
	if err != nil {
		return diag.FromErr(err)
	}

	s, err := service.WaitForCreation(ctx, d, m)
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(schemautil.BuildResourceID(project, s.Name))

	return resourceServiceRead(ctx, d, m)
}

func resourceServiceUpdate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*aiven.Client)

	var karapace *bool
	if v := d.Get("karapace"); d.HasChange("karapace") && v != nil {
		if k, ok := v.(bool); ok && k {
			*karapace = true
		}
	}

	// On service update, we send a default disc space value for a service
	// if the TF user does not specify it
	diskSpace, err := getDefaultDiskSpaceIfNotSet(ctx, d, client)
	if err != nil {
		return diag.FromErr(err)
	}

	projectName, serviceName := schemautil.SplitResourceID2(d.Id())

	if _, err := client.Services.Update(
		projectName,
		serviceName,
		aiven.UpdateServiceRequest{
			Cloud:                 d.Get("cloud_name").(string),
			Plan:                  d.Get("plan").(string),
			MaintenanceWindow:     service.GetMaintenanceWindow(d),
			ProjectVPCID:          service.GetProjectVPCIdPointer(d),
			Powered:               true,
			TerminationProtection: d.Get("termination_protection").(bool),
			DiskSpaceMB:           diskSpace,
			Karapace:              karapace,
			UserConfig:            uconf.ConvertTerraformUserConfigToAPICompatibleFormat("service", d.Get("service_type").(string), false, d),
		},
	); err != nil {
		return diag.FromErr(err)
	}

	if _, err := service.WaitForUpdate(ctx, d, m); err != nil {
		return diag.FromErr(err)
	}

	return resourceServiceRead(ctx, d, m)
}

func getDefaultDiskSpaceIfNotSet(ctx context.Context, d *schema.ResourceData, client *aiven.Client) (int, error) {
	var diskSpace int
	if ds, ok := d.GetOk("disk_space"); !ok {
		// get service plan specific defaults
		servicePlanParams, err := service.GetServicePlanParametersFromSchema(ctx, client, d)
		if err != nil {
			return 0, fmt.Errorf("unable to get service plan parameters: %w", err)
		}

		diskSpace = servicePlanParams.DiskSizeMBDefault
	} else {
		diskSpace = service.ConvertToDiskSpaceMB(ds.(string))
	}

	return diskSpace, nil
}

func resourceServiceDelete(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*aiven.Client)

	projectName, serviceName := schemautil.SplitResourceID2(d.Id())

	if err := client.Services.Delete(projectName, serviceName); err != nil && !aiven.IsNotFound(err) {
		return diag.FromErr(err)
	}
	return nil
}

func resourceServiceState(ctx context.Context, d *schema.ResourceData, m interface{}) ([]*schema.ResourceData, error) {
	client := m.(*aiven.Client)

	if len(strings.Split(d.Id(), "/")) != 2 {
		return nil, fmt.Errorf("invalid identifier %v, expected <project_name>/<service_name>", d.Id())
	}

	projectName, serviceName := schemautil.SplitResourceID2(d.Id())
	s, err := client.Services.Get(projectName, serviceName)
	if err != nil {
		return nil, fmt.Errorf("unable to GET service %s: %s", d.Id(), err)
	}
	servicePlanParams, err := service.GetServicePlanParametersFromServiceResponse(ctx, client, projectName, s)
	if err != nil {
		return nil, fmt.Errorf("unable to get service plan parameters: %w", err)
	}

	err = copyServicePropertiesFromAPIResponseToTerraform(d, s, servicePlanParams, projectName)
	if err != nil {
		return nil, err
	}

	return []*schema.ResourceData{d}, nil
}

func copyServicePropertiesFromAPIResponseToTerraform(
	d *schema.ResourceData,
	s *aiven.Service,
	servicePlanParams service.PlanParameters,
	project string,
) error {
	serviceType := d.Get("service_type").(string)
	if _, ok := d.GetOk("service_type"); !ok {
		serviceType = s.Type
	}

	if err := d.Set("cloud_name", s.CloudName); err != nil {
		return err
	}
	if err := d.Set("service_name", s.Name); err != nil {
		return err
	}
	if err := d.Set("state", s.State); err != nil {
		return err
	}
	if err := d.Set("plan", s.Plan); err != nil {
		return err
	}
	if err := d.Set("service_type", serviceType); err != nil {
		return err
	}
	if err := d.Set("termination_protection", s.TerminationProtection); err != nil {
		return err
	}
	if err := d.Set("maintenance_window_dow", s.MaintenanceWindow.DayOfWeek); err != nil {
		return err
	}
	if err := d.Set("maintenance_window_time", s.MaintenanceWindow.TimeOfDay); err != nil {
		return err
	}
	if err := d.Set("disk_space_used", service.HumanReadableByteSize(s.DiskSpaceMB*units.MiB)); err != nil {
		return err
	}
	if err := d.Set("disk_space_default", service.HumanReadableByteSize(servicePlanParams.DiskSizeMBDefault*units.MiB)); err != nil {
		return err
	}
	if err := d.Set("disk_space_step", service.HumanReadableByteSize(servicePlanParams.DiskSizeMBStep*units.MiB)); err != nil {
		return err
	}
	if err := d.Set("disk_space_cap", service.HumanReadableByteSize(servicePlanParams.DiskSizeMBMax*units.MiB)); err != nil {
		return err
	}
	if err := d.Set("service_uri", s.URI); err != nil {
		return err
	}
	if err := d.Set("project", project); err != nil {
		return err
	}

	if s.ProjectVPCID != nil {
		if err := d.Set("project_vpc_id", schemautil.BuildResourceID(project, *s.ProjectVPCID)); err != nil {
			return err
		}
	}
	userConfig := uconf.ConvertAPIUserConfigToTerraformCompatibleFormat("service", serviceType, s.UserConfig)
	if err := d.Set(serviceType+"_user_config", uconf.NormalizeIpFilter(d.Get(serviceType+"_user_config"), userConfig)); err != nil {
		return fmt.Errorf("cannot set `%s_user_config` : %s; Please make sure that all Aiven services have unique s names", serviceType, err)
	}

	params := s.URIParams
	if err := d.Set("service_host", params["host"]); err != nil {
		return err
	}

	port, _ := strconv.ParseInt(params["port"], 10, 32)
	if err := d.Set("service_port", port); err != nil {
		return err
	}

	password, passwordOK := params["password"]
	username, usernameOK := params["user"]
	if passwordOK {
		if err := d.Set("service_password", password); err != nil {
			return err
		}
	}
	if usernameOK {
		if err := d.Set("service_username", username); err != nil {
			return err
		}
	}

	if err := d.Set("components", flattenServiceComponents(s)); err != nil {
		return fmt.Errorf("cannot set `components` : %s", err)
	}

	return copyConnectionInfoFromAPIResponseToTerraform(d, serviceType, s.ConnectionInfo)
}

func flattenServiceComponents(r *aiven.Service) []map[string]interface{} {
	var components []map[string]interface{}

	for _, c := range r.Components {
		component := map[string]interface{}{
			"component": c.Component,
			"host":      c.Host,
			"port":      c.Port,
			"route":     c.Route,
			"usage":     c.Usage,
		}
		components = append(components, component)
	}

	return components
}

func copyConnectionInfoFromAPIResponseToTerraform(
	d *schema.ResourceData,
	serviceType string,
	connectionInfo aiven.ConnectionInfo,
) error {
	props := make(map[string]interface{})

	switch serviceType {
	case "cassandra":
	case "opensearch":
		props["opensearch_dashboards_uri"] = connectionInfo.OpensearchDashboardsURI
	case "elasticsearch":
		props["kibana_uri"] = connectionInfo.KibanaURI
	case "grafana":
	case "influxdb":
		props["database_name"] = connectionInfo.InfluxDBDatabaseName
	case "kafka":
		props["access_cert"] = connectionInfo.KafkaAccessCert
		props["access_key"] = connectionInfo.KafkaAccessKey
		props["connect_uri"] = connectionInfo.KafkaConnectURI
		props["rest_uri"] = connectionInfo.KafkaRestURI
		props["schema_registry_uri"] = connectionInfo.SchemaRegistryURI
	case "kafka_connect":
	case "mysql":
	case "pg":
		if connectionInfo.PostgresURIs != nil && len(connectionInfo.PostgresURIs) > 0 {
			props["uri"] = connectionInfo.PostgresURIs[0]
		}
		if connectionInfo.PostgresParams != nil && len(connectionInfo.PostgresParams) > 0 {
			params := connectionInfo.PostgresParams[0]
			props["dbname"] = params.DatabaseName
			props["host"] = params.Host
			props["password"] = params.Password
			port, err := strconv.ParseInt(params.Port, 10, 32)
			if err == nil {
				props["port"] = int(port)
			}
			props["sslmode"] = params.SSLMode
			props["user"] = params.User
		}
		props["replica_uri"] = connectionInfo.PostgresReplicaURI
	case "clickhouse":
	case "redis":
	case "flink":
		props["host_ports"] = connectionInfo.FlinkHostPorts
	case "kafka_mirrormaker":
	case "m3db":
	case "m3aggregator":
	default:
		panic(fmt.Sprintf("Unsupported service type %v", serviceType))
	}

	if err := d.Set(serviceType, []map[string]interface{}{props}); err != nil {
		return err
	}

	return nil
}
