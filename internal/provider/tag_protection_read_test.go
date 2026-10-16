package provider

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"codeberg.org/mvdkleijn/forgejo-sdk/forgejo/v3"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestTagProtectionReadDistinguishesMissingAndForbidden(t *testing.T) {
	for _, tc := range []struct {
		name                   string
		repoStatus, ruleStatus int
		missing, failure       bool
	}{
		{"present", 200, 200, false, false},
		{"repository removed", 404, 200, true, false},
		{"repository forbidden", 403, 200, false, true},
		{"rule removed", 200, 404, true, false},
		{"rule forbidden", 200, 403, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				var body string
				switch r.URL.Path {
				case "/api/v1/repositories/1":
					w.WriteHeader(tc.repoStatus)
					body = `{"id":1,"name":"repo","owner":{"login":"org"}}`
				case "/api/v1/repos/org/repo/tag_protections/2":
					w.WriteHeader(tc.ruleStatus)
					body = `{"id":2,"name_pattern":"v*"}`
				default:
					t.Errorf("unexpected path %s", r.URL.Path)
					http.NotFound(w, r)
					return
				}
				if _, err := w.Write([]byte(body)); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			client, err := forgejo.NewClient(server.URL, forgejo.SetForgejoVersion("17.0.0"))
			if err != nil {
				t.Fatal(err)
			}
			r := tagProtectionResource{client: client}
			var schemaResp resource.SchemaResponse
			r.Schema(t.Context(), resource.SchemaRequest{}, &schemaResp)
			state := tfsdk.State{Schema: schemaResp.Schema}
			diags := state.Set(t.Context(), tagProtectionResourceModel{ID: types.Int64Value(2), RepositoryID: types.Int64Value(1), NamePattern: types.StringValue("v*"), WhitelistUsernames: types.SetNull(types.StringType), WhitelistTeams: types.SetNull(types.StringType)})
			if diags.HasError() {
				t.Fatal(diags)
			}
			response := resource.ReadResponse{State: state}
			r.Read(t.Context(), resource.ReadRequest{State: state}, &response)
			if response.Diagnostics.HasError() != tc.failure {
				t.Errorf("unexpected diagnostics: %v", response.Diagnostics)
			}
			if response.State.Raw.IsNull() != tc.missing {
				t.Errorf("state removed=%v; want %v", response.State.Raw.IsNull(), tc.missing)
			}
		})
	}
}
