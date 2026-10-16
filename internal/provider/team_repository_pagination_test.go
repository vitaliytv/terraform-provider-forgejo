package provider

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"codeberg.org/mvdkleijn/forgejo-sdk/forgejo/v3"
)

func TestTeamHasRepositoryFollowsPagination(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("page") == "1" {
			w.Header().Set("Link", fmt.Sprintf(`<http://%s/api/v1/teams/42/repos?page=2>; rel="next"`, r.Host))
			if _, err := w.Write([]byte(`[{"id":1}]`)); err != nil {
				t.Error(err)
			}
		} else if r.URL.Query().Get("page") == "2" {
			if _, err := w.Write([]byte(`[{"id":99}]`)); err != nil {
				t.Error(err)
			}
		} else {
			t.Errorf("unexpected page %q", r.URL.Query().Get("page"))
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := forgejo.NewClient(server.URL, forgejo.SetForgejoVersion("17.0.0"))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		id    int64
		found bool
	}{{99, true}, {100, false}} {
		found, _, err := teamHasRepository(client, 42, tc.id)
		if err != nil || found != tc.found {
			t.Errorf("repository %d: found=%v, err=%v", tc.id, found, err)
		}
	}
}
