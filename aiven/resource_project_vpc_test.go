// Copyright (c) 2017 jelmersnoeck
// Copyright (c) 2018-2021 Aiven, Helsinki, Finland. https://aiven.io/
package aiven

import (
	"fmt"
	"testing"

	"github.com/aiven/aiven-go-client"
	"github.com/aiven/terraform-provider-aiven/aiven/internal/schemautil"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func TestAccAivenProjectVPC_basic(t *testing.T) {
	resourceName := "aiven_project_vpc.bar"
	rName := acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)
	rName2 := acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviderFactories,
		CheckDestroy:      testAccCheckAivenProjectVPCResourceDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccProjectVPCResource(rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckAivenProjectVPCAttributes("data.aiven_project_vpc.vpc"),
					resource.TestCheckResourceAttr(resourceName, "project", fmt.Sprintf("test-acc-pr-%s", rName)),
					resource.TestCheckResourceAttr(resourceName, "cloud_name", "google-europe-west1"),
					resource.TestCheckResourceAttr(resourceName, "network_cidr", "192.168.0.0/24"),
					resource.TestCheckResourceAttr(resourceName, "state", "ACTIVE"),
				),
			},
			{
				Config: testAccProjectVPCCustomTimeoutResource(rName2),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckAivenProjectVPCAttributes("data.aiven_project_vpc.vpc"),
					resource.TestCheckResourceAttr(resourceName, "project", fmt.Sprintf("test-acc-pr-%s", rName2)),
					resource.TestCheckResourceAttr(resourceName, "cloud_name", "google-europe-west1"),
					resource.TestCheckResourceAttr(resourceName, "network_cidr", "192.168.0.0/24"),
					resource.TestCheckResourceAttr(resourceName, "state", "ACTIVE"),
				),
			},
		},
	})
}

func testAccProjectVPCResource(name string) string {
	return fmt.Sprintf(`
		resource "aiven_project" "foo" {
		  project = "test-acc-pr-%s"
		}
		
		resource "aiven_project_vpc" "bar" {
		  project      = aiven_project.foo.project
		  cloud_name   = "google-europe-west1"
		  network_cidr = "192.168.0.0/24"
		}
		
		data "aiven_project_vpc" "vpc" {
		  project    = aiven_project_vpc.bar.project
		  cloud_name = aiven_project_vpc.bar.cloud_name
		
		  depends_on = [aiven_project_vpc.bar]
		}`,
		name)
}

func testAccProjectVPCCustomTimeoutResource(name string) string {
	return fmt.Sprintf(`
		resource "aiven_project" "foo" {
		  project = "test-acc-pr-%s"
		}
		
		resource "aiven_project_vpc" "bar" {
		  project      = aiven_project.foo.project
		  cloud_name   = "google-europe-west1"
		  network_cidr = "192.168.0.0/24"
		
		  timeouts {
		    create = "10m"
		    delete = "5m"
		  }
		}
		
		data "aiven_project_vpc" "vpc" {
		  project    = aiven_project_vpc.bar.project
		  cloud_name = aiven_project_vpc.bar.cloud_name
		
		  depends_on = [aiven_project_vpc.bar]
		}`,
		name)
}

func testAccCheckAivenProjectVPCAttributes(n string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		r := s.RootModule().Resources[n]
		a := r.Primary.Attributes

		if a["project"] == "" {
			return fmt.Errorf("expected to get a project name from Aiven")
		}

		if a["cloud_name"] == "" {
			return fmt.Errorf("expected to get an project user cloud_name from Aiven")
		}

		if a["network_cidr"] == "" {
			return fmt.Errorf("expected to get an project user network_cidr from Aiven")
		}

		if a["state"] == "" {
			return fmt.Errorf("expected to get an project user state from Aiven")
		}

		return nil
	}
}

func testAccCheckAivenProjectVPCResourceDestroy(s *terraform.State) error {
	c := testAccProvider.Meta().(*aiven.Client)

	// loop through the resources in state, verifying each project VPC is destroyed
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "aiven_project_vpc" {
			continue
		}

		projectName, vpcId := schemautil.SplitResourceID2(rs.Primary.ID)
		vpc, err := c.VPCs.Get(projectName, vpcId)
		if err != nil {
			errStatus := err.(aiven.Error).Status
			if errStatus != 404 && errStatus != 403 {
				return err
			}
		}

		if vpc != nil {
			return fmt.Errorf("porject vpc (%s) still exists", rs.Primary.ID)
		}
	}

	return nil
}
