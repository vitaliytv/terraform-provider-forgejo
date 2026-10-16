resource "forgejo_organization" "example" {
  name = "example-org"
}

resource "forgejo_repository" "example" {
  owner = forgejo_organization.example.name
  name  = "example-repo"
}

resource "forgejo_team" "release" {
  organization_id           = forgejo_organization.example.id
  name                      = "release-team"
  includes_all_repositories = true
  permission                = "write"
  units_map = {
    "repo.code" = "write"
  }
}

resource "forgejo_tag_protection" "releases" {
  repository_id   = forgejo_repository.example.id
  name_pattern    = "v*"
  whitelist_teams = [forgejo_team.release.name]
}

# The API requires at least one whitelisted user or team on creation.
# Whitelists can be cleared by a subsequent update to remove all exceptions.
# Import an existing rule using the numeric repository and protection IDs.
import {
  id = "123/42"
  to = forgejo_tag_protection.releases
}
