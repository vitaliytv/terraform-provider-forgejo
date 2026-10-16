resource "forgejo_organization" "example" {
  name = "example-org"
}

resource "forgejo_repository" "example" {
  owner = forgejo_organization.example.name
  name  = "example-repo"
}

resource "forgejo_team" "example" {
  organization_id           = forgejo_organization.example.id
  name                      = "readers"
  includes_all_repositories = false
  permission                = "read"
  units_map = {
    "repo.code" = "read"
  }
}

resource "forgejo_team_repository" "example" {
  team_id       = forgejo_team.example.id
  repository_id = forgejo_repository.example.id
}

# Import an existing association using its numeric team and repository IDs.
import {
  id = "42/123"
  to = forgejo_team_repository.example
}
