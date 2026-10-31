package common

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRemoteRouteServesAppWithoutMaskingMissingAssets(t *testing.T) {
	mux := http.NewServeMux()
	if err := SetupSPAHandler(mux); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path   string
		status int
	}{{"/remote", 200}, {"/assets/missing-ottoman.js", 404}} {
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, httptest.NewRequest("GET", tc.path, nil))
		if response.Code != tc.status {
			t.Errorf("%s: status %d want %d", tc.path, response.Code, tc.status)
		}
	}
}
