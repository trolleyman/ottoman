//go:build windows

package display

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/trolleyman/ottoman/internal/common"
)

// WindowsManager implements display management on Windows
type WindowsManager struct {
	store *Layouts
}

func newPlatformManager(store *Layouts) (Manager, error) {
	return &WindowsManager{store: store}, nil
}

// ListMonitors returns information about connected monitors using PowerShell
func (m *WindowsManager) ListMonitors() ([]MonitorInfo, error) {
	// Use PowerShell to get monitor information via WMI
	script := `
Get-CimInstance -Namespace root\wmi -ClassName WmiMonitorBasicDisplayParams | ForEach-Object {
    $id = $_.InstanceName
    $active = $_.Active

    # Get monitor ID info
    $monId = Get-CimInstance -Namespace root\wmi -ClassName WmiMonitorID | Where-Object { $_.InstanceName -eq $id }

    $manufacturer = if ($monId.ManufacturerName) {
        -join [char[]]($monId.ManufacturerName | Where-Object { $_ -ne 0 })
    } else { "" }

    $name = if ($monId.UserFriendlyName) {
        -join [char[]]($monId.UserFriendlyName | Where-Object { $_ -ne 0 })
    } else { "" }

    Write-Output "$id|$active|$manufacturer|$name"
}
`
	cmd := exec.Command("powershell", "-NoProfile", "-Command", script)
	output, err := cmd.Output()
	if err != nil {
		return nil, errors.Wrap(err, "failed to list monitors")
	}

	var monitors []MonitorInfo
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")

	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.Split(line, "|")
		if len(parts) < 4 {
			continue
		}

		active := parts[1] == "True"

		monitors = append(monitors, MonitorInfo{
			ID:           parts[0],
			Name:         parts[3],
			Manufacturer: parts[2],
			Connected:    active,
			Primary:      i == 0, // First monitor is usually primary
		})
	}

	// Get resolution info separately using a different approach
	if err := m.enrichWithResolutionInfo(monitors); err != nil {
		// Log but don't fail - basic info is still useful
		fmt.Printf("Warning: could not get resolution info: %v\n", err)
	}

	return monitors, nil
}

// enrichWithResolutionInfo adds resolution data to monitor info
func (m *WindowsManager) enrichWithResolutionInfo(monitors []MonitorInfo) error {
	script := `
Add-Type -AssemblyName System.Windows.Forms
[System.Windows.Forms.Screen]::AllScreens | ForEach-Object {
    $name = $_.DeviceName
    $bounds = $_.Bounds
    $primary = $_.Primary
    Write-Output "$name|$($bounds.Width)|$($bounds.Height)|$($bounds.X)|$($bounds.Y)|$primary"
}
`
	cmd := exec.Command("powershell", "-NoProfile", "-Command", script)
	output, err := cmd.Output()
	if err != nil {
		return err
	}

	screenInfo := make(map[string]struct {
		Width, Height, X, Y int
		Primary             bool
	})

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) < 6 {
			continue
		}

		width, _ := strconv.Atoi(parts[1])
		height, _ := strconv.Atoi(parts[2])
		x, _ := strconv.Atoi(parts[3])
		y, _ := strconv.Atoi(parts[4])
		primary := parts[5] == "True"

		screenInfo[parts[0]] = struct {
			Width, Height, X, Y int
			Primary             bool
		}{width, height, x, y, primary}
	}

	// Match up with monitors (best effort)
	for i := range monitors {
		if i < len(screenInfo) {
			// Use index-based matching as a fallback
			for _, info := range screenInfo {
				monitors[i].Width = info.Width
				monitors[i].Height = info.Height
				monitors[i].PositionX = info.X
				monitors[i].PositionY = info.Y
				monitors[i].Primary = info.Primary
				break
			}
		}
	}

	return nil
}

// GetCurrentLayout attempts to identify the current layout
func (m *WindowsManager) GetCurrentLayout() (string, error) {
	monitors, err := m.ListMonitors()
	if err != nil {
		return "", err
	}

	// Try to match current state to a known layout
	names, err := m.store.List()
	if err == nil {
		for _, name := range names {
			layout, _ := m.store.Get(name)
			if matchesLayout(monitors, layout) {
				return name, nil
			}
		}
	}

	return "", nil // No matching layout found
}

// matchesLayout checks if current monitors match a layout
func matchesLayout(monitors []MonitorInfo, layout common.SimplifiedLayout) bool {
	if len(monitors) != len(layout.Monitors) {
		return false
	}

	for _, lm := range layout.Monitors {
		if !lm.Enabled {
			continue
		}
		found := false
		for _, m := range monitors {
			if m.Width == lm.Width && m.Height == lm.Height {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}

// ApplyLayout applies a named layout
func (m *WindowsManager) ApplyLayout(name string) error {
	layout, ok := m.store.Get(name)
	if !ok {
		return errors.Errorf("layout %q not found", name)
	}
	return m.ApplyLayoutConfig(layout)
}

// ApplyLayoutConfig applies a display configuration
// This uses Windows Display Settings or third-party tools
func (m *WindowsManager) ApplyLayoutConfig(layout common.SimplifiedLayout) error {
	// Build PowerShell script to apply display settings
	// Note: Windows doesn't have a simple built-in CLI for this,
	// so we use a combination of approaches

	// First, try using DisplaySwitch for simple cases
	activeCount := 0
	for _, mon := range layout.Monitors {
		if mon.Enabled {
			activeCount++
		}
	}

	switch activeCount {
	case 1:
		// Single display - use DisplaySwitch
		return m.runDisplaySwitch("/internal")
	case 2:
		// Check if it's extend mode
		return m.runDisplaySwitch("/extend")
	}

	// For complex layouts, we need to use SetDisplayConfig API
	// This requires more sophisticated handling
	return m.applyComplexLayout(layout)
}

// runDisplaySwitch uses the built-in DisplaySwitch utility
func (m *WindowsManager) runDisplaySwitch(mode string) error {
	cmd := exec.Command("DisplaySwitch.exe", mode)
	return cmd.Run()
}

// applyComplexLayout applies a multi-monitor layout
// This is a placeholder for more sophisticated display configuration
func (m *WindowsManager) applyComplexLayout(layout common.SimplifiedLayout) error {
	// For complex layouts, we would need to:
	// 1. Use the Windows SetDisplayConfig API via CGO
	// 2. Or use a helper executable like MultiMonitorTool
	// 3. Or use PowerShell with .NET interop

	// For now, provide a PowerShell-based approach using .NET
	script := m.buildLayoutScript(layout)

	cmd := exec.Command("powershell", "-NoProfile", "-Command", script)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return errors.Wrapf(err, "failed to apply layout\nOutput: %s", string(output))
	}

	return nil
}

// buildLayoutScript creates a PowerShell script to apply display settings
func (m *WindowsManager) buildLayoutScript(layout common.SimplifiedLayout) string {
	// This is a simplified version - full implementation would use
	// P/Invoke to call SetDisplayConfig
	var sb strings.Builder

	sb.WriteString(`
$signature = @"
[DllImport("user32.dll")]
public static extern int ChangeDisplaySettingsEx(
    string lpszDeviceName,
    ref DEVMODE lpDevMode,
    IntPtr hwnd,
    uint dwflags,
    IntPtr lParam);
"@
`)

	// For now, just log what we would do
	sb.WriteString(fmt.Sprintf("Write-Host 'Would apply layout: %s'\n", layout.Name))
	for _, mon := range layout.Monitors {
		if mon.Enabled {
			sb.WriteString(fmt.Sprintf(
				"Write-Host '  Monitor %s: %dx%d @ %.0fHz at (%d,%d)'\n",
				mon.Name, mon.Width, mon.Height, mon.RefreshRate,
				mon.PositionX, mon.PositionY,
			))
		}
	}

	return sb.String()
}
