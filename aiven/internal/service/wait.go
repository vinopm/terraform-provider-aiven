// Copyright (c) 2017 jelmersnoeck
// Copyright (c) 2018-2021 Aiven, Helsinki, Finland. https://aiven.io/
package service

import (
	"context"
	"fmt"
	"log"
	"net"
	"strconv"
	"time"

	"github.com/aiven/aiven-go-client"
	"github.com/aiven/terraform-provider-aiven/aiven/internal/schemautil"
	"github.com/aiven/terraform-provider-aiven/aiven/internal/uconf"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

const (
	aivenTargetState           = "RUNNING"
	aivenPendingState          = "REBUILDING"
	aivenRebalancingState      = "REBALANCING"
	aivenServicesStartingState = "WAITING_FOR_SERVICES"
)

func WaitForCreation(ctx context.Context, d *schema.ResourceData, m interface{}) (*aiven.Service, error) {
	return waitForCreateOrUpdate(ctx, d, m, false)
}

func WaitForUpdate(ctx context.Context, d *schema.ResourceData, m interface{}) (*aiven.Service, error) {
	return waitForCreateOrUpdate(ctx, d, m, true)
}

func WaitForDeletion(ctx context.Context, d *schema.ResourceData, m interface{}) error {
	return waitForDeletion(ctx, d, m)
}

func waitForCreateOrUpdate(ctx context.Context, d *schema.ResourceData, m interface{}, isUpdate bool) (*aiven.Service, error) {
	client := m.(*aiven.Client)

	projectName, serviceName := d.Get("project").(string), d.Get("service_name").(string)

	timeout := d.Timeout(schema.TimeoutCreate)
	if isUpdate {
		log.Printf("[DEBUG] Service update waiter timeout %.0f minutes", timeout.Minutes())
		timeout = d.Timeout(schema.TimeoutUpdate)
	} else {
		log.Printf("[DEBUG] Service create waiter timeout %.0f minutes", timeout.Minutes())
	}

	conf := &resource.StateChangeConf{
		Pending:                   []string{aivenPendingState, aivenRebalancingState, aivenServicesStartingState},
		Target:                    []string{aivenTargetState},
		Delay:                     10 * time.Second,
		Timeout:                   timeout,
		MinTimeout:                2 * time.Second,
		ContinuousTargetOccurence: 5,
		Refresh: func() (interface{}, string, error) {
			service, err := client.Services.Get(projectName, serviceName)
			if err != nil {
				return nil, "", fmt.Errorf("unable to fetch service from api: %w", err)
			}

			state := service.State
			if isUpdate {
				// When updating service don't wait for it to enter RUNNING state because that can take
				// very long time if for example service plan or cloud it runs in is changed and the
				// service has a lot of data. If the service was already previously in RUNNING state we
				// can manage the associated resources even if the service is rebuilding.
				state = aivenTargetState
			}

			if state != aivenTargetState {
				log.Printf("[DEBUG] service reports as %s, still for it to be in state %s", state, aivenTargetState)
				return service, state, nil
			}

			if rdy := backupsReady(service); !rdy {
				log.Printf("[DEBUG] service reports as %s, still waiting for service backups", state)
				return service, aivenServicesStartingState, nil
			}

			if rdy := grafanaReady(service); !rdy {
				log.Printf("[DEBUG] service reports as %s, still waiting for grafana", state)
				return service, aivenServicesStartingState, nil
			}

			if rdy, err := staticIpsReady(d, m); err != nil {
				return nil, "", fmt.Errorf("unable to check if static ips are ready: %w", err)
			} else if !rdy {
				log.Printf("[DEBUG] service reports as %s, still waiting for static ips", state)
				return service, aivenServicesStartingState, nil
			}
			return service, state, nil
		},
	}

	aux, err := conf.WaitForStateContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to wait for service state change: %w", err)
	}
	return aux.(*aiven.Service), nil
}

func waitForDeletion(ctx context.Context, d *schema.ResourceData, m interface{}) error {
	client := m.(*aiven.Client)

	projectName, serviceName := d.Get("project").(string), d.Get("service_name").(string)

	timeout := d.Timeout(schema.TimeoutDelete)
	log.Printf("[DEBUG] Service deletion waiter timeout %.0f minutes", timeout.Minutes())

	conf := &resource.StateChangeConf{
		Pending:    []string{"deleting"},
		Target:     []string{"deleted"},
		Delay:      10 * time.Second,
		Timeout:    timeout,
		MinTimeout: 20 * time.Second,
		Refresh: func() (interface{}, string, error) {
			_, err := client.Services.Get(projectName, serviceName)
			if err != nil && !aiven.IsNotFound(err) {
				return nil, "", fmt.Errorf("unable to check if service is gone: %w", err)
			}

			log.Printf("[DEBUG] service gone, still waiting for static ips to be disassociated")

			if dis, err := staticIpsDisassociated(d, m); err != nil {
				return nil, "", fmt.Errorf("unable to check if static ips are disassociated: %w", err)
			} else if !dis {
				return struct{}{}, "deleting", nil
			}

			return struct{}{}, "deleted", nil
		},
	}

	if _, err := conf.WaitForStateContext(ctx); err != nil {
		return fmt.Errorf("unable to wait for service deletion: %w", err)
	}
	return nil
}

func grafanaReady(service *aiven.Service) bool {
	if service.Type != "grafana" {
		return true
	}

	// if IP filter is anything but 0.0.0.0/0 skip Grafana service availability checks
	ipFilters, ok := service.UserConfig["ip_filter"]
	if ok {
		f := ipFilters.([]interface{})
		if len(f) > 1 {
			log.Printf("[DEBUG] grafana serivce has `%+v` ip filters, and availability checks will be skipped", ipFilters)

			return true
		}

		if len(f) == 1 {
			if f[0] != "0.0.0.0/0" {
				log.Printf("[DEBUG] grafana serivce has `%+v` ip filters, and availability checks will be skipped", ipFilters)

				return true
			}
		}
	}

	var publicGrafana string

	// constructing grafana public address if available
	for _, component := range service.Components {
		if component.Route == "public" && component.Usage == "primary" {
			publicGrafana = component.Host + ":" + strconv.Itoa(component.Port)
			continue
		}
	}

	// checking if public grafana is reachable
	if publicGrafana != "" {
		_, err := net.DialTimeout("tcp", publicGrafana, 1*time.Second)
		if err != nil {
			log.Printf("[DEBUG] public grafana is not yet reachable")
			return false
		}

		log.Printf("[DEBUG] public grafana is reachable")
		return true
	}

	return true
}

func backupsReady(service *aiven.Service) bool {
	if service.Type != "pg" && service.Type != "elasticsearch" &&
		service.Type != "redis" && service.Type != "influxdb" {
		return true
	}

	// no backups for read replicas type of service
	for _, i := range service.Integrations {
		if i.IntegrationType == "read_replica" && *i.DestinationService == service.Name {
			return true
		}
	}

	return len(service.Backups) > 0
}

func staticIpsReady(d *schema.ResourceData, m interface{}) (bool, error) {
	if !staticIpsEnabledInUserConfig(d) {
		return true, nil
	}

	expectedStaticIps := staticIpsForServiceFromSchema(d)
	if len(expectedStaticIps) == 0 {
		return true, nil
	}

	client := m.(*aiven.Client)
	projectName, serviceName := d.Get("project").(string), d.Get("service_name").(string)

	staticIpsList, err := client.StaticIPs.List(projectName)
	if err != nil {
		return false, fmt.Errorf("unable to fetch static ips for project '%s': '%w", projectName, err)
	}

L:
	for i := range expectedStaticIps {
		eip := expectedStaticIps[i]
		for j := range staticIpsList.StaticIPs {
			sip := staticIpsList.StaticIPs[j]

			assignedOrAvailable := sip.State == "assigned" || sip.State == "available"
			belongsToService := sip.ServiceName == serviceName
			isExpectedIp := sip.StaticIPAddressID == eip

			if isExpectedIp && belongsToService && assignedOrAvailable {
				continue L
			}
		}
		return false, nil
	}

	return true, nil
}

func staticIpsDisassociated(d *schema.ResourceData, m interface{}) (bool, error) {
	if !staticIpsEnabledInUserConfig(d) {
		return true, nil
	}

	expectedStaticIps := staticIpsForServiceFromSchema(d)
	if len(expectedStaticIps) == 0 {
		return true, nil
	}

	client := m.(*aiven.Client)
	projectName := d.Get("project").(string)

	staticIpsList, err := client.StaticIPs.List(projectName)
	if err != nil {
		return false, fmt.Errorf("unable to fetch static ips for project '%s': '%w", projectName, err)
	}

	for i := range expectedStaticIps {
		eip := expectedStaticIps[i]
		for j := range staticIpsList.StaticIPs {
			sip := staticIpsList.StaticIPs[j]

			// no check for service name since after deletion the field is gone, but the
			// static ip lingers in the assigned state for a while until it gets 'created'
			// again
			ipIsAssigned := sip.State == "assigned"
			isExpectedIp := sip.StaticIPAddressID == eip

			if isExpectedIp && ipIsAssigned {
				return false, nil
			}
		}
	}
	return true, nil
}

func staticIpsForServiceFromSchema(d *schema.ResourceData) []string {
	return schemautil.FlattenToString(d.Get("static_ips").([]interface{}))
}

func staticIpsEnabledInUserConfig(d *schema.ResourceData) bool {
	cfg := uconf.ConvertTerraformUserConfigToAPICompatibleFormat("service", d.Get("service_type").(string), false, d)
	if v, ok := cfg["static_ips"]; !ok {
		return false
	} else {
		return v.(bool)
	}
}
