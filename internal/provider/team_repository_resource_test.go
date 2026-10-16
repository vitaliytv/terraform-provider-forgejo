package provider_test

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"testing"

	"codeberg.org/mvdkleijn/forgejo-sdk/forgejo/v3"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccTeamRepositoryResource(t *testing.T) {
	var teamID int64
	config := func(target string) string {
		base := providerConfig + `
resource "forgejo_organization" "binding" { name = "test_team_repo_org" }
resource "forgejo_repository" "first" {
  owner = forgejo_organization.binding.name
  name = "first"
  private = true
}
resource "forgejo_repository" "second" {
  owner = forgejo_organization.binding.name
  name = "second"
  private = true
}
resource "forgejo_team" "readers" {
  organization_id = forgejo_organization.binding.id
  name = "readers"
  includes_all_repositories = false
  permission = "read"
  units_map = { "repo.code" = "read" }
}
resource "forgejo_user" "reader" {
  depends_on = [forgejo_team.readers, forgejo_repository.first, forgejo_repository.second]
  login = "test_team_repo_user"
  email = "test_team_repo_user@localhost.localdomain"
  password = "Local-Fixture-Pass123!"
  must_change_password = false
}
resource "forgejo_team_member" "reader" {
  team_id = forgejo_team.readers.id
  user = forgejo_user.reader.login
}
`
		if target != "" {
			base += fmt.Sprintf(`
resource "forgejo_team_repository" "binding" {
  depends_on = [forgejo_team_member.reader]
  team_id = forgejo_team.readers.id
  repository_id = forgejo_repository.%s.id
}
`, target)
		}
		return base
	}
	checkAccess := func(first, second bool) resource.TestCheckFunc {
		return func(s *terraform.State) error {
			var err error
			teamID, err = strconv.ParseInt(s.RootModule().Resources["forgejo_team.readers"].Primary.Attributes["id"], 10, 64)
			if err != nil {
				return err
			}
			admin, err := forgejo.NewClient(forgejoTestHost, forgejo.SetToken(os.Getenv("FORGEJO_API_TOKEN")))
			if err != nil {
				return err
			}
			reader, err := forgejo.NewClient(forgejoTestHost, forgejo.SetBasicAuth("test_team_repo_user", "Local-Fixture-Pass123!"))
			if err != nil {
				return err
			}
			for name, allowed := range map[string]bool{"first": first, "second": second} {
				if _, _, err := admin.GetRepo("test_team_repo_org", name); err != nil {
					return fmt.Errorf("repository must still exist: %w", err)
				}
				_, result, err := reader.GetRepo("test_team_repo_org", name)
				if allowed && err != nil {
					return fmt.Errorf("reader cannot access assigned repository %s: %w", name, err)
				}
				if !allowed && (err == nil || result == nil || (result.StatusCode != 403 && result.StatusCode != 404)) {
					return fmt.Errorf("reader must not have access to unassigned repository %s", name)
				}
			}
			return nil
		}
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: config("first"), Check: checkAccess(true, false)},
			{ResourceName: "forgejo_team_repository.binding", ImportState: true, ImportStateId: "0/42", ExpectError: regexp.MustCompile("positive numeric IDs")},
			{
				ResourceName: "forgejo_team_repository.binding", ImportState: true, ImportStateVerify: true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					return s.RootModule().Resources["forgejo_team_repository.binding"].Primary.Attributes["id"], nil
				},
			},
			{
				PreConfig: func() {
					client, err := forgejo.NewClient(forgejoTestHost, forgejo.SetToken(os.Getenv("FORGEJO_API_TOKEN")))
					if err != nil {
						t.Fatal(err)
					}
					if _, err := client.RemoveTeamRepository(teamID, "test_team_repo_org", "first"); err != nil {
						t.Fatal(err)
					}
				},
				Config: config("first"), Check: checkAccess(true, false),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction("forgejo_team_repository.binding", plancheck.ResourceActionCreate)}},
			},
			{
				Config: config("second"), Check: checkAccess(false, true),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction("forgejo_team_repository.binding", plancheck.ResourceActionReplace)}},
			},
			{Config: config(""), Check: checkAccess(false, false)},
		},
	})
}

func TestAccTeamRepositoryRejectsImplicitAccess(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: providerConfig + `
resource "forgejo_organization" "implicit" { name = "test_team_repo_implicit" }
resource "forgejo_repository" "implicit" {
  owner = forgejo_organization.implicit.name
  name = "implicit"
}
resource "forgejo_team" "implicit" {
  organization_id = forgejo_organization.implicit.id
  name = "all-repositories"
  includes_all_repositories = true
  permission = "read"
  units_map = { "repo.code" = "read" }
}
resource "forgejo_team_repository" "implicit" {
  team_id = forgejo_team.implicit.id
  repository_id = forgejo_repository.implicit.id
}
`,
			ExpectError: regexp.MustCompile("Team includes all repositories"),
		}},
	})
}
