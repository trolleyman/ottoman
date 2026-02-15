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
	"github.com/trolleyman/ottoman/internal/api"
)

// LinuxManager implements display management on Linux using xrandr
type LinuxManager struct {
	store *Layouts
}

func newPlatformManager(store *Layouts) (Manager, error) {
	// Check if xrandr is available
	if _, err := exec.LookPath("xrandr"); err != nil {
		return nil, errors.Wrap(err, "xrandr not found")
	}
	return &LinuxManager{store: store}, nil
}

// ListMonitors returns information about connected monitors
func (m *LinuxManager) ListMonitors() ([]api.Monitor, error) {
	cmd := exec.Command("xrandr", "--query")
	output, err := cmd.Output()
	if err != nil {
		return nil, errors.Wrap(err, "xrandr query failed")
	}

	return parseXrandrOutput(string(output))
}

// parseXrandrOutput parses xrandr --query output
func parseXrandrOutput(output string) ([]api.Monitor, error) {
	var monitors []api.Monitor

	// Regex patterns
	// Matches: "DP-1 connected primary 2560x1440+0+0 (normal left inverted right x axis y axis) 597mm x 336mm"
	outputPattern := regexp.MustCompile(`^(\S+)\s+(connected|disconnected)\s*(primary)?\s*(\d+x\d+\+\d+\+\d+)?`)
	// Matches: "   2560x1440     59.95*+  143.91    119.99"
	modePattern := regexp.MustCompile(`^\s+(\d+)x(\d+)\s+([\d.]+)(\*)?(\+)?`)

	scanner := bufio.NewScanner(strings.NewReader(output))
	var currentMonitor *api.Monitor
	var currentActive *api.ActiveMonitor

	for scanner.Scan() {
		line := scanner.Text()

		// Check for output line
		if matches := outputPattern.FindStringSubmatch(line); len(matches) > 0 {
			// Save previous monitor if any
			if currentMonitor != nil {
				if currentActive != nil {
					currentMonitor.Active = currentActive
				}
				monitors = append(monitors, *currentMonitor)
			}

			port := matches[1]
			connected := matches[2] == "connected"
			primary := matches[3] == "primary"

			currentMonitor = &api.Monitor{
				Edid:         "",   // Not available from xrandr
				Port:         port,
				Name:         port, // Use port as name on Linux
				Manufacturer: "",   // Not available from xrandr
			}
			currentActive = nil

			if connected {
				currentActive = &api.ActiveMonitor{
					Primary: primary,
					Model:   "", // Not available from xrandr
				}
			}

			// Parse geometry if present (e.g., "2560x1440+0+0")
			if currentActive != nil && len(matches) > 4 && matches[4] != "" {
				geom := matches[4]
				parts := strings.Split(geom, "+")
				if len(parts) >= 3 {
					res := strings.Split(parts[0], "x")
					if len(res) == 2 {
						currentActive.Width, _ = strconv.Atoi(res[0])
						currentActive.Height, _ = strconv.Atoi(res[1])
					}
					currentActive.PositionX, _ = strconv.Atoi(parts[1])
					currentActive.PositionY, _ = strconv.Atoi(parts[2])
				}
			}
			continue
		}

		// Check for mode line (resolution and refresh rate)
		if currentActive != nil && currentActive.RefreshRate == 0 {
			if matches := modePattern.FindStringSubmatch(line); len(matches) > 0 {
				// Check if this is the active mode (marked with *)
				if len(matches) > 4 && matches[4] == "*" {
					width, _ := strconv.Atoi(matches[1])
					height, _ := strconv.Atoi(matches[2])
					refreshRate, _ := strconv.ParseFloat(matches[3], 64)

					// Only update if not already set from geometry
					if currentActive.Width == 0 {
						currentActive.Width = width
						currentActive.Height = height
					}
					currentActive.RefreshRate = int(refreshRate)
				}
			}
		}
	}

	// Don't forget the last monitor
	if currentMonitor != nil {
		if currentActive != nil {
			currentMonitor.Active = currentActive
		}
		monitors = append(monitors, *currentMonitor)
	}

	return monitors, nil
}

// ApplyLayoutConfig applies a display configuration using xrandr
func (m *LinuxManager) ApplyLayoutConfig(layout api.Layout) error {
	args := m.buildXrandrArgs(layout)
	cmd := exec.Command("xrandr", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return errors.Wrapf(err, "xrandr failed\nOutput: %s", string(output))
	}
	return nil
}

// buildXrandrArgs builds xrandr command arguments for a layout
func (m *LinuxManager) buildXrandrArgs(layout api.Layout) []string {
	var args []string

	for _, mon := range layout.Monitors {
		// Use Port for xrandr output name
		outputName := mon.Port
		if outputName == "" {
			continue // Skip monitors without port specification
		}

		args = append(args, "--output", outputName)

		// TODO: Add --off for monitors that are currently connected but shouldn't be
		// if !mon.Enabled {
		// 	args = append(args, "--off")
		// 	continue
		// }

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
