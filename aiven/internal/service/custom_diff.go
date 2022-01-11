// Copyright (c) 2018-2021 Aiven, Helsinki, Finland. https://aiven.io/

package service

import (
	"context"
	"fmt"

	"github.com/aiven/aiven-go-client"
	"github.com/docker/go-units"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func ServiceIntegrationShouldNotBeEmpty(_ context.Context, _, new, _ interface{}) bool {
	return len(new.([]interface{})) != 0
}

func CustomizeDiffServiceIntegrationAfterCreation(_ context.Context, d *schema.ResourceDiff, _ interface{}) error {
	if len(d.Id()) > 0 && d.HasChange("service_integrations") && len(d.Get("service_integrations").([]interface{})) != 0 {
		return fmt.Errorf("service_integrations field can only be set during creation of a service")
	}
	return nil
}

func DiskSpaceShouldNotBeEmpty(_ context.Context, _, new, _ interface{}) bool {
	return new.(string) != ""
}

func CustomizeDiffCheckDiskSpace(ctx context.Context, d *schema.ResourceDiff, m interface{}) error {
	client := m.(*aiven.Client)

	if d.Get("service_type").(string) == "" {
		return fmt.Errorf("cannot check dynamic disc space because service_type is empty")
	}

	servicePlanParams, err := GetServicePlanParametersFromSchema(ctx, client, d)
	if err != nil {
		return fmt.Errorf("unable to get service plan parameters: %w", err)
	}

	var requestedDiskSpaceMB int
	ds, okDiskSpace := d.GetOk("disk_space")
	if !okDiskSpace {
		return nil
	}

	requestedDiskSpaceMB = ConvertToDiskSpaceMB(ds.(string))

	if servicePlanParams.DiskSizeMBDefault != requestedDiskSpaceMB {
		// first check if the plan allows dynamic disk sizing
		if servicePlanParams.DiskSizeMBMax == 0 || servicePlanParams.DiskSizeMBStep == 0 {
			return fmt.Errorf("dynamic disk space is not configurable for this service")
		}

		// next check if the cloud allows it by checking the pricing per gb
		if ok, err := dynamicDiskSpaceIsAllowedByPricing(ctx, client, d); err != nil {
			return fmt.Errorf("unable to check if dynamic disk space is allowed for this service: %w", err)
		} else if !ok {
			return fmt.Errorf("dynamic disk space is not configurable for this service")
		}
	}

	humanReadableDiskSpaceDefault := HumanReadableByteSize(servicePlanParams.DiskSizeMBDefault * units.MiB)
	humanReadableDiskSpaceMax := HumanReadableByteSize(servicePlanParams.DiskSizeMBMax * units.MiB)
	humanReadableDiskSpaceStep := HumanReadableByteSize(servicePlanParams.DiskSizeMBStep * units.MiB)
	humanReadableRequestedDiskSpace := HumanReadableByteSize(requestedDiskSpaceMB * units.MiB)

	if requestedDiskSpaceMB < servicePlanParams.DiskSizeMBDefault {
		return fmt.Errorf("requested disk size is too small: '%s' < '%s'", humanReadableRequestedDiskSpace, humanReadableDiskSpaceDefault)
	}
	if servicePlanParams.DiskSizeMBMax != 0 {
		if requestedDiskSpaceMB > servicePlanParams.DiskSizeMBMax {
			return fmt.Errorf("requested disk size is too large: '%s' > '%s'", humanReadableRequestedDiskSpace, humanReadableDiskSpaceMax)
		}
	}
	if servicePlanParams.DiskSizeMBStep != 0 {
		if (requestedDiskSpaceMB-servicePlanParams.DiskSizeMBDefault)%servicePlanParams.DiskSizeMBStep != 0 {
			return fmt.Errorf("requested disk size has to increase from: '%s' in increments of '%s'", humanReadableDiskSpaceDefault, humanReadableDiskSpaceStep)
		}
	}
	return nil
}

func SetServiceTypeIfEmpty(t string) schema.CustomizeDiffFunc {
	return func(ctx context.Context, diff *schema.ResourceDiff, i interface{}) error {
		if err := diff.SetNew("service_type", t); err != nil {
			return err
		}

		return nil
	}
}
