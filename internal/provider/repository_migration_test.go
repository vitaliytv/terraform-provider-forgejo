package provider_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// Exercise schema validation and the actual SDK request without requiring an
// external migration source or sending source credentials to a remote service.
func TestAccRepositoryMigrationRequest(t *testing.T) {
	for _, service := range []string{"onedev", "gitbucket", "codebase", "pagure", "bitbucketdc"} {
		t.Run(service, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/api/v1/version":
					if err := json.NewEncoder(w).Encode(map[string]string{"version": "17.0.0"}); err != nil {
						t.Error(err)
					}
				case r.Method == http.MethodPost && r.URL.Path == "/api/v1/repos/migrate":
					calls.Add(1)
					var body struct {
						Service      string `json:"service"`
						AuthUsername string `json:"auth_username"`
						AuthPassword string `json:"auth_password"`
						AuthToken    string `json:"auth_token"`
					}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					if body.Service != service || body.AuthUsername != "fixture-user" || body.AuthPassword != "fixture-password" || body.AuthToken != "" {
						t.Error("migration service or source credentials did not reach the API as configured")
					}
					w.WriteHeader(http.StatusUnprocessableEntity)
					if err := json.NewEncoder(w).Encode(map[string]string{"message": "migration request received by fixture"}); err != nil {
						t.Error(err)
					}
				default:
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{{
					Config: fmt.Sprintf(`
provider "forgejo" {
  host      = %q
  username  = ""
  password  = ""
  api_token = "fixture-api-token"
}
resource "forgejo_repository" "migration" {
  name          = "migration-request-test"
  owner         = "fixture-owner"
  clone_addr    = "https://example.com/source/repository.git"
  service       = %q
  auth_username = "fixture-user"
  auth_password = "fixture-password"
}
`, server.URL, service),
					ExpectError: regexp.MustCompile("migration request received by fixture"),
				}},
			})
			if calls.Load() == 0 && !t.Skipped() {
				t.Error("migration API was never called")
			}
		})
	}
}
