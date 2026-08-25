package agent

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/trolleyman/ottoman/internal/api"
	"github.com/trolleyman/ottoman/internal/display"
)

func TestLayoutsExportUsesCachedState(t *testing.T) {
	// No display manager is installed: a regression that probes the backend
	// would panic, while the cached export has everything the mirror needs.
	a := &Agent{
		layouts: display.NewLayoutsFromSlice([]api.Layout{
			{Id: "desk", Name: "Desk", Aliases: []string{"2"}},
			{Id: "tv", Name: "TV", Aliases: []string{"1"}},
		}),
		currentLayout: "tv",
	}
	w := httptest.NewRecorder()
	a.handleLayoutsExport(w, httptest.NewRequest(http.MethodGet, "/api/layouts/export", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var got api.LayoutsResponse
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.CurrentLayout != "tv" {
		t.Errorf("current layout = %q, want tv", got.CurrentLayout)
	}
	if len(got.Layouts) != 2 || got.Layouts[0].Id != "tv" || got.Layouts[1].Id != "desk" {
		t.Errorf("layouts = %+v, want alias-sorted cached layouts", got.Layouts)
	}
}
