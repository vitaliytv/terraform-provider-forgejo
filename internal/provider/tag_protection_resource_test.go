package provider_test

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"codeberg.org/mvdkleijn/forgejo-sdk/forgejo/v3"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccTagProtectionResource(t *testing.T) {
	var ruleID int64
	config := func(pattern, users, teams string) string {
		base := providerConfig + `
resource "forgejo_organization" "tags" { name = "test_protected_tags_org" }
resource "forgejo_repository" "tags" {
  owner = forgejo_organization.tags.name
  name = "protected-tags"
}
resource "forgejo_team" "release" {
  depends_on = [forgejo_repository.tags]
  organization_id = forgejo_organization.tags.id
  name = "release-team"
  includes_all_repositories = true
  permission = "write"
  units_map = { "repo.code" = "write" }
}
`
		if pattern != "" {
			base += fmt.Sprintf(`
resource "forgejo_tag_protection" "tags" {
  repository_id = forgejo_repository.tags.id
  name_pattern = %q
  whitelist_usernames = %s
  whitelist_teams = %s
  depends_on = [forgejo_team.release]
}
`, pattern, users, teams)
		}
		return base
	}
	checkTagCreation := func(name string, allowed bool, preserveTag string) resource.TestCheckFunc {
		return func(s *terraform.State) error {
			client, err := forgejo.NewClient(forgejoTestHost, forgejo.SetToken(os.Getenv("FORGEJO_API_TOKEN")))
			if err != nil {
				return err
			}
			if state, ok := s.RootModule().Resources["forgejo_tag_protection.tags"]; ok {
				ruleID, err = strconv.ParseInt(state.Primary.Attributes["id"], 10, 64)
				if err != nil {
					return err
				}
			}
			_, result, err := client.CreateTag("test_protected_tags_org", "protected-tags", forgejo.CreateTagOption{TagName: name, Target: "main"})
			if allowed && err != nil {
				return fmt.Errorf("allowed tag creation failed: %w", err)
			}
			if !allowed && (err == nil || result == nil || (result.StatusCode != 403 && result.StatusCode != 422) || !strings.Contains(strings.ToLower(err.Error()), "protected")) {
				return fmt.Errorf("tag %s must be rejected by tag protection, got: %v", name, err)
			}
			if preserveTag != "" {
				if _, _, err := client.GetTag("test_protected_tags_org", "protected-tags", preserveTag); err != nil {
					return fmt.Errorf("existing tag must be preserved: %w", err)
				}
			}
			return nil
		}
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config("v*", "[]", `["release-team"]`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("forgejo_tag_protection.tags", "name_pattern", "v*"),
					resource.TestCheckResourceAttr("forgejo_tag_protection.tags", "whitelist_usernames.#", "0"),
					resource.TestCheckResourceAttr("forgejo_tag_protection.tags", "whitelist_teams.#", "1"),
					checkTagCreation("v-denied", false, ""),
				),
			},
			{ResourceName: "forgejo_tag_protection.tags", ImportState: true, ImportStateId: "bad/1", ExpectError: regexp.MustCompile("positive numeric IDs")},
			{
				ResourceName: "forgejo_tag_protection.tags", ImportState: true, ImportStateVerify: true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					attrs := s.RootModule().Resources["forgejo_tag_protection.tags"].Primary.Attributes
					return attrs["repository_id"] + "/" + attrs["id"], nil
				},
			},
			{
				Config:           config("v*", `["tfadmin"]`, `["release-team"]`),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction("forgejo_tag_protection.tags", plancheck.ResourceActionUpdate)}},
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("forgejo_tag_protection.tags", "whitelist_usernames.#", "1"),
					resource.TestCheckResourceAttr("forgejo_tag_protection.tags", "whitelist_teams.#", "1"),
					checkTagCreation("v-allowed", true, ""),
				),
			},
			{
				Config:           config("release-*", "[]", "[]"),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction("forgejo_tag_protection.tags", plancheck.ResourceActionUpdate)}},
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("forgejo_tag_protection.tags", "name_pattern", "release-*"),
					resource.TestCheckResourceAttr("forgejo_tag_protection.tags", "whitelist_usernames.#", "0"),
					resource.TestCheckResourceAttr("forgejo_tag_protection.tags", "whitelist_teams.#", "0"),
					checkTagCreation("release-denied", false, "v-allowed"),
				),
			},
			{
				PreConfig: func() {
					client, err := forgejo.NewClient(forgejoTestHost, forgejo.SetToken(os.Getenv("FORGEJO_API_TOKEN")))
					if err != nil {
						t.Fatal(err)
					}
					if _, err := client.DeleteTagProtection("test_protected_tags_org", "protected-tags", ruleID); err != nil {
						t.Fatal(err)
					}
				},
				Config:           config("release-*", "[]", `["release-team"]`),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction("forgejo_tag_protection.tags", plancheck.ResourceActionCreate)}},
				Check:            checkTagCreation("release-recreated-denied", false, "v-allowed"),
			},
			{Config: config("", "[]", "[]"), Check: checkTagCreation("release-unprotected", true, "v-allowed")},
		},
	})
}
