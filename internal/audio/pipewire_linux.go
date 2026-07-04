//go:build linux

package audio

import (
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/trolleyman/ottoman/internal/common"
)

// pipewireController controls audio via PipeWire's wpctl CLI.
type pipewireController struct{}

func newPlatformController() (Controller, error) {
	if _, err := exec.LookPath("wpctl"); err != nil {
		return nil, errors.Wrap(err, "wpctl not found (PipeWire/WirePlumber required)")
	}
	return &pipewireController{}, nil
}

// sinkLine matches a sink entry in `wpctl status`, e.g.
//
//	│  *   55. HDA NVidia Digital Stereo (HDMI) [vol: 0.65]
//	│      61. Logi Z407                        [vol: 1.00]
//
// capturing the optional default marker, the id, and the description.
var sinkLineRe = regexp.MustCompile(`(\*)?\s*(\d+)\.\s+(.+)`)

// ListSinks parses `wpctl status` for the Sinks section, then enriches each
// sink with its stable node.name and current volume/mute state.
func (c *pipewireController) ListSinks() ([]Sink, error) {
	out, err := common.RunCmdOutput("wpctl", "status")
	if err != nil {
		return nil, errors.Wrap(err, "wpctl status failed")
	}

	sinks := parseSinkSection(out)
	for i := range sinks {
		if name, desc, err := inspectNode(sinks[i].ID); err == nil {
			if name != "" {
				sinks[i].Name = name
			}
			if desc != "" {
				sinks[i].Description = desc
			}
		}
		if vol, muted, err := getVolume(sinks[i].ID); err == nil {
			sinks[i].Volume = vol
			sinks[i].Muted = muted
		}
	}
	return sinks, nil
}

// parseSinkSection extracts sinks from `wpctl status` output. It reads lines in
// the "Sinks:" subsection of the Audio block, stopping at the next subsection.
func parseSinkSection(status string) []Sink {
	var sinks []Sink
	inSinks := false

	for _, raw := range strings.Split(status, "\n") {
		// Strip the tree-drawing prefix characters so matching is stable.
		line := strings.Trim(raw, " │├└─╰╭╠╟\t")

		if strings.HasPrefix(line, "Sinks:") {
			inSinks = true
			continue
		}
		if !inSinks {
			continue
		}
		// Any other "Word:" header ends the sinks subsection.
		if isSectionHeader(line) {
			break
		}

		m := sinkLineRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		id, err := strconv.ParseUint(m[2], 10, 32)
		if err != nil {
			continue
		}
		desc := strings.TrimSpace(m[3])
		// Drop a trailing "[vol: x.xx]" annotation if present.
		if idx := strings.LastIndex(desc, "[vol:"); idx >= 0 {
			desc = strings.TrimSpace(desc[:idx])
		}
		sinks = append(sinks, Sink{
			ID:          uint32(id),
			Description: desc,
			Default:     m[1] == "*",
		})
	}
	return sinks
}

// isSectionHeader reports whether a (prefix-stripped) line is a wpctl category
// header like "Sources:" or "Filters:".
func isSectionHeader(line string) bool {
	if !strings.HasSuffix(line, ":") {
		return false
	}
	// A header has no digits (sink/source entries always start "N.").
	return !strings.ContainsAny(line, "0123456789")
}

var (
	nodeNameRe = regexp.MustCompile(`node\.name\s*=\s*"([^"]+)"`)
	nodeDescRe = regexp.MustCompile(`node\.description\s*=\s*"([^"]+)"`)
)

// inspectNode returns the node.name and node.description for a PipeWire id.
func inspectNode(id uint32) (name, desc string, err error) {
	out, err := common.RunCmdOutput("wpctl", "inspect", strconv.FormatUint(uint64(id), 10))
	if err != nil {
		return "", "", errors.Wrap(err, "wpctl inspect failed")
	}
	if m := nodeNameRe.FindStringSubmatch(out); m != nil {
		name = m[1]
	}
	if m := nodeDescRe.FindStringSubmatch(out); m != nil {
		desc = m[1]
	}
	return name, desc, nil
}

var volumeRe = regexp.MustCompile(`Volume:\s*([\d.]+)`)

// getVolume parses `wpctl get-volume <id>` (e.g. "Volume: 0.65 [MUTED]").
func getVolume(id uint32) (volume float64, muted bool, err error) {
	out, err := common.RunCmdOutput("wpctl", "get-volume", strconv.FormatUint(uint64(id), 10))
	if err != nil {
		return 0, false, errors.Wrap(err, "wpctl get-volume failed")
	}
	return parseVolume(out)
}

func parseVolume(out string) (volume float64, muted bool, err error) {
	muted = strings.Contains(out, "[MUTED]") || strings.Contains(strings.ToUpper(out), "MUTED")
	m := volumeRe.FindStringSubmatch(out)
	if m == nil {
		return 0, muted, errors.Errorf("could not parse volume from %q", strings.TrimSpace(out))
	}
	v, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0, muted, errors.Wrap(err, "invalid volume value")
	}
	return v, muted, nil
}

// resolveID finds the current PipeWire id for a stable node name.
func (c *pipewireController) resolveID(name string) (uint32, error) {
	sinks, err := c.ListSinks()
	if err != nil {
		return 0, err
	}
	for _, s := range sinks {
		if s.Name == name {
			return s.ID, nil
		}
	}
	return 0, errors.Errorf("no sink with node name %q", name)
}

func (c *pipewireController) SetVolume(name string, volume float64) error {
	if volume < 0 {
		volume = 0
	}
	if volume > 1.5 {
		volume = 1.5
	}
	id, err := c.resolveID(name)
	if err != nil {
		return err
	}
	// wpctl accepts a plain float ratio (1.0 = 100%).
	arg := strconv.FormatFloat(volume, 'f', 3, 64)
	if err := common.RunCmd("wpctl", "set-volume", strconv.FormatUint(uint64(id), 10), arg); err != nil {
		return errors.Wrap(err, "wpctl set-volume failed")
	}
	return nil
}

func (c *pipewireController) SetMute(name string, muted bool) error {
	id, err := c.resolveID(name)
	if err != nil {
		return err
	}
	state := "0"
	if muted {
		state = "1"
	}
	if err := common.RunCmd("wpctl", "set-mute", strconv.FormatUint(uint64(id), 10), state); err != nil {
		return errors.Wrap(err, "wpctl set-mute failed")
	}
	return nil
}

func (c *pipewireController) SetDefault(name string) error {
	id, err := c.resolveID(name)
	if err != nil {
		return err
	}
	if err := common.RunCmd("wpctl", "set-default", strconv.FormatUint(uint64(id), 10)); err != nil {
		return errors.Wrap(err, "wpctl set-default failed")
	}
	return nil
}
