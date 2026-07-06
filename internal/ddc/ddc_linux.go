//go:build linux

package ddc

import (
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/trolleyman/ottoman/internal/common"
)

// Available reports whether the ddcutil CLI is installed.
func Available() bool {
	_, err := exec.LookPath("ddcutil")
	return err == nil
}

// Detect lists DDC/CI-capable displays. It is comparatively slow (probes each
// i2c bus), so callers should cache the result.
func Detect() ([]Display, error) {
	out, err := common.RunCmdOutput("ddcutil", "detect")
	if err != nil {
		return nil, errors.Wrap(err, "ddcutil detect failed")
	}
	return parseDetect(out), nil
}

var (
	busRe    = regexp.MustCompile(`/dev/i2c-(\d+)`)
	mfgRe    = regexp.MustCompile(`Mfg id:\s+(\S+)`)
	modelRe  = regexp.MustCompile(`Model:\s+(.+?)\s*$`)
	serialRe = regexp.MustCompile(`Serial number:\s+(.+?)\s*$`)
)

// parseDetect parses `ddcutil detect` output into displays. Each "Display N"
// block yields one entry (invalid/unaddressable ones are skipped).
func parseDetect(out string) []Display {
	var displays []Display
	var cur *Display

	flush := func() {
		if cur != nil && cur.Bus >= 0 {
			displays = append(displays, *cur)
		}
		cur = nil
	}

	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(line)

		// A new "Display N" (or "Invalid display") block header starts at column 0.
		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") && trimmed != "" {
			flush()
			if strings.HasPrefix(trimmed, "Display") {
				cur = &Display{Bus: -1}
			}
			continue
		}
		if cur == nil {
			continue
		}

		if m := busRe.FindStringSubmatch(line); m != nil {
			if n, err := strconv.Atoi(m[1]); err == nil {
				cur.Bus = n
			}
		}
		if m := mfgRe.FindStringSubmatch(line); m != nil {
			cur.Mfg = m[1]
		}
		if m := modelRe.FindStringSubmatch(line); m != nil && strings.Contains(line, "Model:") {
			cur.Model = strings.TrimSpace(m[1])
		}
		if m := serialRe.FindStringSubmatch(line); m != nil && strings.Contains(line, "Serial number:") {
			cur.Serial = strings.TrimSpace(m[1])
		}
	}
	flush()
	return displays
}

var vcpValueRe = regexp.MustCompile(`current value =\s*(\d+),\s*max value =\s*(\d+)`)

// GetBrightness returns the current brightness as a 0-100 percentage.
func GetBrightness(bus int) (int, error) {
	out, err := common.RunCmdOutput("ddcutil", "--bus", strconv.Itoa(bus), "getvcp", vcpBrightness)
	if err != nil {
		return 0, errors.Wrap(err, "ddcutil getvcp brightness failed")
	}
	cur, max, err := parseVCPValue(out)
	if err != nil {
		return 0, err
	}
	if max <= 0 {
		max = 100
	}
	return cur * 100 / max, nil
}

func parseVCPValue(out string) (cur, max int, err error) {
	m := vcpValueRe.FindStringSubmatch(out)
	if m == nil {
		return 0, 0, errors.Errorf("could not parse VCP value from %q", strings.TrimSpace(out))
	}
	cur, _ = strconv.Atoi(m[1])
	max, _ = strconv.Atoi(m[2])
	return cur, max, nil
}

// SetBrightness sets brightness as a 0-100 percentage.
//
// `--noverify` skips ddcutil's default post-write read-back, which otherwise
// doubles the DDC/CI traffic (a full getvcp after every setvcp) and its
// spec-mandated inter-operation sleeps — the main source of drag lag. We don't
// need the confirmation: the UI polls brightness separately, and a dropped
// write is corrected by the next drag tick.
func SetBrightness(bus, percent int) error {
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	if err := common.RunCmd("ddcutil", "--bus", strconv.Itoa(bus), "--noverify", "setvcp", vcpBrightness, strconv.Itoa(percent)); err != nil {
		return errors.Wrap(err, "ddcutil setvcp brightness failed")
	}
	return nil
}

// SetPower turns the display on (0xD6=1) or to standby (0xD6=4).
func SetPower(bus int, on bool) error {
	value := powerStandby
	if on {
		value = powerOn
	}
	if err := common.RunCmd("ddcutil", "--bus", strconv.Itoa(bus), "setvcp", vcpPower, strconv.Itoa(value)); err != nil {
		return errors.Wrap(err, "ddcutil setvcp power failed")
	}
	return nil
}
