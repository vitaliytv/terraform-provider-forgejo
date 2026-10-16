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

func TestTeamRepositoryReadDistinguishesMissingAndForbidden(t *testing.T) {
	for _, tc := range []struct {
		name                       string
		teamStatus, listStatus     int
		implicit, missing, failure bool
	}{
		{"present", 200, 200, false, false, false},
		{"team removed", 404, 200, false, true, false},
		{"team forbidden", 403, 200, false, false, true},
		{"association missing", 200, 200, false, true, false},
		{"list forbidden", 200, 403, false, false, true},
		{"implicit access", 200, 200, true, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				var body string
				switch r.URL.Path {
				case "/api/v1/teams/42":
					w.WriteHeader(tc.teamStatus)
					body = `{"id":42,"includes_all_repositories":false}`
					if tc.implicit {
						body = `{"id":42,"includes_all_repositories":true}`
					}
				case "/api/v1/teams/42/repos":
					w.WriteHeader(tc.listStatus)
					body = `[{"id":99}]`
					if tc.missing {
						body = `[]`
					}
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
			r := teamRepositoryResource{client: client}
			var schemaResp resource.SchemaResponse
			r.Schema(t.Context(), resource.SchemaRequest{}, &schemaResp)
			state := tfsdk.State{Schema: schemaResp.Schema}
			diags := state.Set(t.Context(), teamRepositoryResourceModel{ID: types.StringValue("42/99"), TeamID: types.Int64Value(42), RepositoryID: types.Int64Value(99)})
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
