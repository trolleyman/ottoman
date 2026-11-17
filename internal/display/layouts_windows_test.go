package display

import (
	"encoding/json"
	"github.com/trolleyman/ottoman/internal/api"
	"os"
	"testing"
)

// Run on the target desktop with its exported layouts. SDC_VALIDATE does not switch displays.
func TestWindowsSavedLayouts(t *testing.T) {
	path := os.Getenv("OTTOMAN_VALIDATE_LAYOUTS")
	if path == "" {
		t.Skip("requires the target displays and their saved layouts")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var layouts []api.Layout
	if err := json.Unmarshal(data, &layouts); err != nil {
		t.Fatal(err)
	}
	if len(layouts) == 0 {
		t.Fatal("no layouts to validate")
	}
	for _, l := range layouts {
		t.Run(l.Name, func(t *testing.T) {
			if err := setWindowsLayout(l, SDC_VALIDATE); err != nil {
				t.Fatal(err)
			}
		})
	}
}
