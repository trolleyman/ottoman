//go:build linux

package ddc

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

// Native EDID-based display detection, replacing `ddcutil detect` for bus
// discovery. ddcutil's detect probes every i2c bus with the DDC/CI handshake
// (address 0x37); a monitor that doesn't answer — e.g. a TV controlled out of
// band over the network — makes it retry for seconds, churning the bus and
// stuttering the compositor. We instead read the EDID at address 0x50 (a quick,
// no-retry 128-byte read that succeeds even on a DDC/CI-mute panel) on the GPU
// display buses only, and parse it into the same Display the matcher consumes.
//
// This deliberately probes only GPU DDC channels: an 0x50 read on an SMBus would
// hit DIMM SPD EEPROMs, so system buses (SMBus/PIIX4/DesignWare) are excluded.

// edidAddr is the fixed i2c slave address of a display's EDID EEPROM.
const edidAddr = 0x50

// edidLen is the length of the EDID base block, which carries everything we
// match on (manufacturer, product name, serial). Extension blocks are ignored.
const edidLen = 128

// DetectDirect discovers DDC displays by reading each GPU display bus's EDID
// over i2c, with no ddcutil and no DDC/CI probe. Buses that don't answer (an
// unplugged connector) are simply skipped.
func DetectDirect() []Display {
	var displays []Display
	for _, bus := range displayBuses() {
		raw, err := readEDID(bus)
		if err != nil {
			continue // no display on this bus (disconnected), or unreadable
		}
		if d, ok := parseEDID(raw); ok {
			d.Bus = bus
			displays = append(displays, d)
		}
	}
	return displays
}

// displayBuses returns the i2c bus numbers that are GPU display DDC channels.
// Two signals are unioned: a DRM connector that exposes its bus via sysfs
// (.../ddc/i2c-dev/i2c-N, as AMD/Intel do), and an adapter whose name matches a
// known GPU display controller (NVIDIA, which does not expose the sysfs link).
// System buses are never included — see the package note on SPD EEPROMs.
func displayBuses() []int {
	set := map[int]bool{}

	// DRM-exposed DDC buses (AMD/Intel).
	for _, link := range globQuiet("/sys/class/drm/*/ddc/i2c-dev/i2c-*") {
		if n, ok := i2cNum(filepath.Base(link)); ok {
			set[n] = true
		}
	}
	// Adapter-name allowlist (covers NVIDIA, which lacks the DRM link).
	for _, dev := range globQuiet("/sys/bus/i2c/devices/i2c-*") {
		name, err := os.ReadFile(filepath.Join(dev, "name"))
		if err != nil {
			continue
		}
		if isDisplayAdapter(string(name)) {
			if n, ok := i2cNum(filepath.Base(dev)); ok {
				set[n] = true
			}
		}
	}

	out := make([]int, 0, len(set))
	for n := range set {
		out = append(out, n)
	}
	sort.Ints(out)
	return out
}

// isDisplayAdapter reports whether an i2c adapter name is a GPU display DDC
// channel. It matches GPU controllers and, crucially, excludes system buses
// (SMBus/PIIX4/DesignWare) where a 0x50 read would collide with SPD EEPROMs.
func isDisplayAdapter(name string) bool {
	n := strings.ToLower(name)
	return strings.Contains(n, "nvidia i2c") ||
		strings.Contains(n, "amdgpu dm i2c") ||
		strings.Contains(n, "i915 gmbus") // Intel
}

func globQuiet(pattern string) []string {
	matches, _ := filepath.Glob(pattern)
	return matches
}

// i2cNum extracts N from an "i2c-N" device/link base name.
func i2cNum(base string) (int, bool) {
	s := strings.TrimPrefix(base, "i2c-")
	if s == base {
		return 0, false
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, false
	}
	return n, true
}

// readEDID reads a display's 128-byte EDID base block from address 0x50 on the
// given bus. It writes the word offset (0) then reads the block — the standard
// EDID access pattern, which GPU DDC buses support directly. The transfer runs
// under the bus lock (i2cBus.once) so it can't interleave with DDC/CI traffic
// on the same wire.
func readEDID(bus int) ([]byte, error) {
	buf := make([]byte, edidLen)
	err := getBus(bus).once(edidAddr, func(f *os.File) error {
		if _, err := f.Write([]byte{0x00}); err != nil {
			return errors.Wrap(err, "write EDID offset")
		}
		if _, err := io.ReadFull(f, buf); err != nil {
			return errors.Wrap(err, "read EDID")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return buf, nil
}

// SysfsDisplays lists the displays the kernel reports as connected, reading
// each DRM connector's EDID from sysfs. Unlike DetectDirect this touches no i2c
// bus at all — the kernel caches the EDID it read when the link came up — so it
// keeps answering when a live 0x50 read doesn't. That happens: a DisplayPort
// monitor tunnels DDC over the AUX channel, and the bridge can stop passing raw
// i2c through after a sleep/wake cycle while the panel stays connected, lit, and
// perfectly willing to take DDC/CI commands.
//
// It's identity only. The connector's DDC bus is resolved from its sysfs ddc
// link when the driver publishes one (AMD/Intel); NVIDIA doesn't, so those come
// back with Bus == -1 and can't be addressed on this information alone.
func SysfsDisplays() []Display {
	var displays []Display
	for _, dir := range globQuiet("/sys/class/drm/card*-*") {
		status, err := os.ReadFile(filepath.Join(dir, "status"))
		if err != nil || strings.TrimSpace(string(status)) != "connected" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, "edid"))
		if err != nil {
			continue
		}
		d, ok := parseEDID(raw)
		if !ok {
			continue // a connected-but-EDID-less connector (or a stub block)
		}
		d.Bus = sysfsDDCBus(dir)
		displays = append(displays, d)
	}
	return displays
}

// sysfsDDCBus returns the i2c bus a DRM connector exposes its DDC channel on,
// or -1 when the driver doesn't publish one.
func sysfsDDCBus(connectorDir string) int {
	for _, link := range globQuiet(filepath.Join(connectorDir, "ddc/i2c-dev/i2c-*")) {
		if n, ok := i2cNum(filepath.Base(link)); ok {
			return n
		}
	}
	return -1
}

var edidHeader = []byte{0x00, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x00}

// parseEDID extracts the identity fields from an EDID base block, formatted to
// match how GNOME/Mutter (and ddcutil) render them, so the result compares
// equal to the registry's "vendor:product:serial" identifier via ddcMatches:
//   - Mfg:    the 3-letter PNP manufacturer id (bytes 8-9).
//   - Model:  the monitor-name descriptor (tag 0xFC).
//   - Serial: the text serial descriptor (tag 0xFF) if present, else the binary
//     serial number (bytes 12-15) formatted as "0x%08x".
func parseEDID(raw []byte) (Display, bool) {
	if len(raw) < edidLen {
		return Display{}, false
	}
	for i, b := range edidHeader {
		if raw[i] != b {
			return Display{}, false
		}
	}

	d := Display{Bus: -1, Mfg: pnpID(raw[8], raw[9])}

	var nameDesc, serialDesc string
	for _, off := range []int{54, 72, 90, 108} {
		desc := raw[off : off+18]
		// A display (non-timing) descriptor is 00 00 00 <tag> 00 <13 bytes>.
		if desc[0] != 0 || desc[1] != 0 || desc[2] != 0 || desc[4] != 0 {
			continue
		}
		switch desc[3] {
		case 0xFC:
			nameDesc = descText(desc[5:18])
		case 0xFF:
			serialDesc = descText(desc[5:18])
		}
	}

	d.Model = nameDesc
	if serialDesc != "" {
		d.Serial = serialDesc
	} else if bin := uint32(raw[12]) | uint32(raw[13])<<8 | uint32(raw[14])<<16 | uint32(raw[15])<<24; bin != 0 {
		d.Serial = fmt.Sprintf("0x%08x", bin)
	}
	return d, true
}

// pnpID decodes the 3-letter manufacturer id packed into two EDID bytes (five
// bits per letter, 1 => 'A').
func pnpID(hi, lo byte) string {
	code := uint16(hi)<<8 | uint16(lo)
	letter := func(v uint16) byte { return byte('A' - 1 + v) }
	return string([]byte{
		letter((code >> 10) & 0x1F),
		letter((code >> 5) & 0x1F),
		letter(code & 0x1F),
	})
}

// descText reads an EDID descriptor's ASCII payload, which is terminated by a
// 0x0A newline and space-padded.
func descText(b []byte) string {
	if i := indexByte(b, 0x0A); i >= 0 {
		b = b[:i]
	}
	return strings.TrimSpace(string(b))
}

func indexByte(b []byte, c byte) int {
	for i, x := range b {
		if x == c {
			return i
		}
	}
	return -1
}
