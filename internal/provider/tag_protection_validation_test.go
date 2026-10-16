package provider_test

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccTagProtectionRequiresWhitelistOnCreation(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: providerConfig + `
resource "forgejo_repository" "empty_whitelist" { name = "test_empty_tag_whitelist" }
resource "forgejo_tag_protection" "empty_whitelist" {
  repository_id = forgejo_repository.empty_whitelist.id
  name_pattern = "v*"
}
`,
			ExpectError: regexp.MustCompile("Empty whitelist on tag protection creation"),
		}},
	})
}
