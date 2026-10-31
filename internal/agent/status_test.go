package agent

import (
	"encoding/json"
	"runtime"
	"testing"
	"time"
)

func TestStatusReportsRemoteOperatingSystem(t *testing.T) {
	a := &Agent{config: &Config{ListenAddress: "127.0.0.1:17294"}, startTime: time.Now()}
	response, err := a.getStatusResponse()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(data, &body); err != nil {
		t.Fatal(err)
	}
	if body["os"] != runtime.GOOS {
		t.Fatalf("status os = %v, want agent GOOS %s", body["os"], runtime.GOOS)
	}
}
