//go:build linux

package display

import (
	"testing"
)

func TestParseXrandrOutput(t *testing.T) {
	input := `Screen 0: minimum 8 x 8, current 4480 x 1440, maximum 32767 x 32767
HDMI-0 connected (normal left inverted right x axis y axis)
   3840x2160     60.00 +  59.94    50.00    29.97    25.00    23.98
   4096x2160     59.94    50.00    29.97    25.00    24.00    23.98
   2560x1440     59.95
   1920x1080    119.88   100.00    60.00    59.94    50.00    29.97    25.00    23.98
   1280x1024     60.02
   1280x720      59.94    50.00
   1152x864      60.00
   1024x768      60.00
   800x600       60.32
   720x576       50.00
   720x480       59.94
   640x480       59.95    59.94    59.93
DP-0 connected primary 2560x1440+1920+0 (normal left inverted right x axis y axis) 597mm x 336mm
   2560x1440     60.00*+ 180.00   170.00   165.00   144.00   120.00
   1920x1080    119.88    60.00    59.94    50.00
   1280x1440     59.91
   1280x1024     75.02    60.02
   1280x720      59.94    50.00
   1024x768     119.99    99.97    75.03    70.07    60.00
   800x600      119.97    99.66    75.00    72.19    60.32    56.25
   720x576       50.00
   720x480       59.94
   640x480      119.52    99.77    75.00    72.81    59.94    59.93
DP-1 disconnected (normal left inverted right x axis y axis)
DP-2 connected 1920x1080+0+360 (normal left inverted right x axis y axis) 527mm x 296mm
   1920x1080     60.00*+  74.97    59.94    50.00
   1680x1050     59.95
   1440x900      59.89
   1280x1024     75.02    60.02
   1280x960      60.00
   1280x720      60.00    59.94    50.00
   1024x768      75.03    70.07    60.00
   800x600       75.00    72.19    60.32    56.25
   720x576       50.00
   720x480       59.94
   640x480       75.00    72.81    59.94    59.93
DP-3 disconnected (normal left inverted right x axis y axis)
DP-4 disconnected (normal left inverted right x axis y axis)
DP-5 disconnected (normal left inverted right x axis y axis)
HDMI-A-1-0 disconnected (normal left inverted right x axis y axis)
DisplayPort-1-0 disconnected (normal left inverted right x axis y axis)
DisplayPort-1-1 disconnected (normal left inverted right x axis y axis)
DisplayPort-1-2 disconnected (normal left inverted right x axis y axis)
`

	monitors, err := parseXrandrOutput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(monitors) != 3 {
		t.Fatalf("expected 3 monitors, got %d", len(monitors))
	}

	// HDMI-0 (inactive)
	if monitors[0].Port != "HDMI-0" || monitors[0].Active != nil {
		t.Errorf("expected HDMI-0 to be inactive, got %v", monitors[0])
	}

	// DP-0 (active)
	if monitors[1].Port != "DP-0" || monitors[1].Active == nil || !monitors[1].Active.Primary || monitors[1].Active.Width != 2560 {
		t.Errorf("expected DP-0 to be active primary 2560x1440, got %v", monitors[1])
	}

	// DP-2 (active)
	if monitors[2].Port != "DP-2" || monitors[2].Active == nil || monitors[2].Active.Primary || monitors[2].Active.Width != 1920 {
		t.Errorf("expected DP-2 to be active non-primary 1920x1080, got %v", monitors[2])
	}
}
