//go:build linux

package display

import (
	"bufio"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/trolleyman/ottoman/internal/common"
)

// LinuxManager implements display management on Linux using xrandr
type LinuxManager struct {
}

func newPlatformManager(store *Layouts) (Manager, error) {
	// Check if xrandr is available
	if _, err := exec.LookPath("xrandr"); err != nil {
		return nil, errors.Wrap(err, "xrandr not found")
	}
	return &LinuxManager{store: store}, nil
}

// ListMonitors returns information about connected monitors
func (m *LinuxManager) ListMonitors() ([]MonitorInfo, error) {
	cmd := exec.Command("xrandr", "--query")
	output, err := cmd.Output()
	if err != nil {
		return nil, errors.Wrap(err, "xrandr query failed")
	}

	return parseXrandrOutput(string(output))
}

// parseXrandrOutput parses xrandr --query output
func parseXrandrOutput(output string) ([]MonitorInfo, error) {
	var monitors []MonitorInfo

	// Regex patterns
	// Matches: "DP-1 connected primary 2560x1440+0+0 (normal left inverted right x axis y axis) 597mm x 336mm"
	outputPattern := regexp.MustCompile(`^(\S+)\s+(connected|disconnected)\s*(primary)?\s*(\d+x\d+\+\d+\+\d+)?`)
	// Matches: "   2560x1440     59.95*+  143.91    119.99"
	modePattern := regexp.MustCompile(`^\s+(\d+)x(\d+)\s+([\d.]+)(\*)?(\+)?`)

	scanner := bufio.NewScanner(strings.NewReader(output))
	var currentMonitor *MonitorInfo

	for scanner.Scan() {
		line := scanner.Text()

		// Check for output line
		if matches := outputPattern.FindStringSubmatch(line); len(matches) > 0 {
			// Save previous monitor if any
			if currentMonitor != nil {
				monitors = append(monitors, *currentMonitor)
			}

			name := matches[1]
			connected := matches[2] == "connected"
			primary := matches[3] == "primary"

			currentMonitor = &MonitorInfo{
				ID:        name,
				Name:      name,
				Connected: connected,
				Primary:   primary,
			}

			// Parse geometry if present (e.g., "2560x1440+0+0")
			if len(matches) > 4 && matches[4] != "" {
				geom := matches[4]
				parts := strings.Split(geom, "+")
				if len(parts) >= 3 {
					res := strings.Split(parts[0], "x")
					if len(res) == 2 {
						currentMonitor.Width, _ = strconv.Atoi(res[0])
						currentMonitor.Height, _ = strconv.Atoi(res[1])
					}
					currentMonitor.PositionX, _ = strconv.Atoi(parts[1])
					currentMonitor.PositionY, _ = strconv.Atoi(parts[2])
				}
			}
			continue
		}

		// Check for mode line (resolution and refresh rate)
		if currentMonitor != nil && currentMonitor.RefreshRate == 0 {
			if matches := modePattern.FindStringSubmatch(line); len(matches) > 0 {
				// Check if this is the active mode (marked with *)
				if len(matches) > 4 && matches[4] == "*" {
					width, _ := strconv.Atoi(matches[1])
					height, _ := strconv.Atoi(matches[2])
					refreshRate, _ := strconv.ParseFloat(matches[3], 64)

					// Only update if not already set from geometry
					if currentMonitor.Width == 0 {
						currentMonitor.Width = width
						currentMonitor.Height = height
					}
					currentMonitor.RefreshRate = refreshRate
				}
			}
		}
	}

	// Don't forget the last monitor
	if currentMonitor != nil {
		monitors = append(monitors, *currentMonitor)
	}

	return monitors, nil
}

// GetCurrentLayout attempts to identify the current layout
func (m *LinuxManager) GetCurrentLayout() (string, error) {
	monitors, err := m.ListMonitors()
	if err != nil {
		return "", err
	}

	// Try to match current state to a known layout
	id, err := m.store.List()
	if err == nil {
		for _, id := range id {
			layout, _ := m.store.Get(id)
			if m.matchesLayout(monitors, layout) {
				return id, nil
			}
		}
	}

	return "", nil
}

// matchesLayout checks if current monitors match a layout
func (m *LinuxManager) matchesLayout(monitors []MonitorInfo, layout common.SimplifiedLayout) bool {
	enabledCount := 0
	for _, lm := range layout.Monitors {
		if lm.Enabled {
			enabledCount++
		}
	}

	connectedCount := 0
	for _, mon := range monitors {
		if mon.Connected && mon.Width > 0 {
			connectedCount++
		}
	}

	if connectedCount != enabledCount {
		return false
	}

	// Check resolutions match
	for _, lm := range layout.Monitors {
		if !lm.Enabled {
			continue
		}
		found := false
		for _, mon := range monitors {
			if mon.Width == lm.Width && mon.Height == lm.Height {
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
func (m *LinuxManager) ApplyLayout(name string) error {
	layout, ok := m.store.Get(name)
	if !ok {
		return errors.Errorf("layout %q not found", name)
	}
	return m.ApplyLayoutConfig(layout)
}

// ApplyLayoutConfig applies a display configuration using xrandr
func (m *LinuxManager) ApplyLayoutConfig(layout common.SimplifiedLayout) error {
	args := m.buildXrandrArgs(layout)
	cmd := exec.Command("xrandr", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return errors.Wrapf(err, "xrandr failed\nOutput: %s", string(output))
	}
	return nil
}

// buildXrandrArgs builds xrandr command arguments for a layout
func (m *LinuxManager) buildXrandrArgs(layout common.SimplifiedLayout) []string {
	var args []string

	for _, mon := range layout.Monitors {
		args = append(args, "--output", mon.Name)

		if !mon.Enabled {
			args = append(args, "--off")
			continue
		}

		// Set mode (resolution)
		mode := fmt.Sprintf("%dx%d", mon.Width, mon.Height)
		args = append(args, "--mode", mode)

		// Set refresh rate if specified
		if mon.RefreshRate > 0 {
			args = append(args, "--rate", fmt.Sprintf("%.2f", mon.RefreshRate))
		}

		// Set position
		pos := fmt.Sprintf("%dx%d", mon.PositionX, mon.PositionY)
		args = append(args, "--pos", pos)

		// Set as primary if needed
		if mon.Primary {
			args = append(args, "--primary")
		}
	}

	return args
}

// GetAvailableModes returns available modes for a monitor
func (m *LinuxManager) GetAvailableModes(monitorName string) ([]ModeInfo, error) {
	cmd := exec.Command("xrandr", "--query")
	output, err := cmd.Output()
	if err != nil {
		return nil, errors.Wrap(err, "xrandr query failed")
	}

	return parseMonitorModes(string(output), monitorName)
}

// ModeInfo represents a display mode
type ModeInfo struct {
	Width       int
	Height      int
	RefreshRate float64
	Current     bool
	Preferred   bool
}

// parseMonitorModes extracts available modes for a specific monitor
func parseMonitorModes(output string, monitorName string) ([]ModeInfo, error) {
	var modes []ModeInfo
	inMonitor := false

	outputPattern := regexp.MustCompile(`^(\S+)\s+(connected|disconnected)`)
	modePattern := regexp.MustCompile(`^\s+(\d+)x(\d+)\s+(.+)`)
	ratePattern := regexp.MustCompile(`([\d.]+)(\*)?(\+)?`)

	scanner := bufio.NewScanner(strings.NewReader(output))

	for scanner.Scan() {
		line := scanner.Text()

		// Check for output line
		if matches := outputPattern.FindStringSubmatch(line); len(matches) > 0 {
			inMonitor = matches[1] == monitorName
			continue
		}

		// Parse mode lines
		if inMonitor {
			if matches := modePattern.FindStringSubmatch(line); len(matches) > 0 {
				width, _ := strconv.Atoi(matches[1])
				height, _ := strconv.Atoi(matches[2])
				ratesStr := matches[3]

				// Parse all refresh rates on this line
				rateMatches := ratePattern.FindAllStringSubmatch(ratesStr, -1)
				for _, rm := range rateMatches {
					rate, _ := strconv.ParseFloat(rm[1], 64)
					modes = append(modes, ModeInfo{
						Width:       width,
						Height:      height,
						RefreshRate: rate,
						Current:     rm[2] == "*",
						Preferred:   rm[3] == "+",
					})
				}
			}
		}
	}

	return modes, nil
}
