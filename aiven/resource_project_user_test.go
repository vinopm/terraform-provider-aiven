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

func TestAccAivenProjectUser_basic(t *testing.T) {
	resourceName := "aiven_project_user.bar"
	rName := acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviderFactories,
		CheckDestroy:      testAccCheckAivenProjectUserResourceDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccProjectUserResource(rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckAivenProjectUserAttributes("data.aiven_project_user.user"),
					resource.TestCheckResourceAttr(resourceName, "project", fmt.Sprintf("test-acc-pr-%s", rName)),
					resource.TestCheckResourceAttr(resourceName, "email", fmt.Sprintf("ivan.savciuc+%s@aiven.fi", rName)),
					resource.TestCheckResourceAttr(resourceName, "member_type", "admin"),
				),
			},
		},
	})
}

func testAccCheckAivenProjectUserResourceDestroy(s *terraform.State) error {
	c := testAccProvider.Meta().(*aiven.Client)

	// loop through the resources in state, verifying each project is destroyed
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "aiven_project_user" {
			continue
		}

		projectName, email := schemautil.SplitResourceID2(rs.Primary.ID)
		p, i, err := c.ProjectUsers.Get(projectName, email)
		if err != nil {
			errStatus := err.(aiven.Error).Status
			if errStatus != 404 && errStatus != 403 {
				return err
			}
		}

		if p != nil {
			return fmt.Errorf("porject user (%s) still exists", rs.Primary.ID)
		}

		if i != nil {
			return fmt.Errorf("porject user invitation (%s) still exists", rs.Primary.ID)
		}
	}

	return nil
}

func testAccProjectUserResource(name string) string {
	return fmt.Sprintf(`
		resource "aiven_project" "foo" {
		  project          = "test-acc-pr-%s"
		  default_cloud    = "aws-eu-west-2"
		  billing_currency = "EUR"
		}
		
		resource "aiven_project_user" "bar" {
		  project     = aiven_project.foo.project
		  email       = "ivan.savciuc+%s@aiven.fi"
		  member_type = "admin"
		}
		
		data "aiven_project_user" "user" {
		  project = aiven_project_user.bar.project
		  email   = aiven_project_user.bar.email
		
		  depends_on = [aiven_project_user.bar]
		}`,
		name, name)
}

func testAccCheckAivenProjectUserAttributes(n string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		r := s.RootModule().Resources[n]
		a := r.Primary.Attributes

		if a["project"] == "" {
			return fmt.Errorf("expected to get a project name from Aiven")
		}

		if a["email"] == "" {
			return fmt.Errorf("expected to get an project user email from Aiven")
		}

		if a["member_type"] == "" {
			return fmt.Errorf("expected to get an project user member_type from Aiven")
		}

		return nil
	}
}
