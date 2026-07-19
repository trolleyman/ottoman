//go:build linux

package display

import (
	"encoding/xml"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

// GNOME Mutter persists the display configuration in monitors.xml under the
// user config dir. Applying a layout over D-Bus with the TEMPORARY method takes
// effect immediately with no "Keep these settings?" confirmation dialog, but
// Mutter does not write it to disk, so it would be lost on the next login. To
// make an Ottoman layout survive a logout/reboot we write the same
// configuration into monitors.xml ourselves, matching Mutter 46's own writer so
// that Mutter reads it back on the next hardware change or session start.
//
// A monitors.xml file can hold several <configuration> blocks, one per distinct
// set of connected monitors. Mutter selects the block whose set of monitor
// specs (enabled + disabled) exactly equals the currently connected set. We
// therefore replace only the block matching the current hardware and preserve
// any others verbatim.

// persistLogicalMonitor is one enabled monitor's placement, captured while
// building the D-Bus apply request so it can also be written to monitors.xml.
type persistLogicalMonitor struct {
	spec    monitorSpec
	x, y    int32
	width   int32
	height  int32
	rate    float64
	scale   float64
	primary bool
}

// --- Parsing types: just enough of monitors.xml to identify each existing
// <configuration> by its monitor-spec set, while keeping its raw inner XML so
// unrelated blocks round-trip losslessly. ---

type monitorsFileXML struct {
	XMLName xml.Name         `xml:"monitors"`
	Version string           `xml:"version,attr"`
	Configs []configEntryXML `xml:"configuration"`
}

type configEntryXML struct {
	Inner         string    `xml:",innerxml"`
	LogicalSpecs  []specXML `xml:"logicalmonitor>monitor>monitorspec"`
	DisabledSpecs []specXML `xml:"disabled>monitorspec"`
}

type specXML struct {
	Connector string `xml:"connector"`
	Vendor    string `xml:"vendor"`
	Product   string `xml:"product"`
	Serial    string `xml:"serial"`
}

// specKey uniquely identifies a monitor across a config's spec set.
func specKey(s monitorSpec) string {
	return s.Connector + "\x00" + s.Vendor + "\x00" + s.Product + "\x00" + s.Serial
}

func specKeyXML(s specXML) string {
	return s.Connector + "\x00" + s.Vendor + "\x00" + s.Product + "\x00" + s.Serial
}

// monitorsXMLPath returns the path Mutter reads, honouring XDG_CONFIG_HOME.
func monitorsXMLPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", errors.Wrap(err, "locating user config dir")
	}
	return filepath.Join(dir, "monitors.xml"), nil
}

// writeMonitorsXML persists the just-applied layout to monitors.xml so it
// survives a reboot. enabled describes the monitors turned on by the layout;
// connected is every physically connected monitor (any not in the layout is
// written as <disabled> so the block still matches the current hardware).
func writeMonitorsXML(enabled []persistLogicalMonitor, connected []mutterMonitor) error {
	// Every connected monitor not enabled by the layout must be listed as
	// disabled, otherwise Mutter's spec-set match against the hardware fails and
	// it ignores our block.
	enabledKeys := make(map[string]bool, len(enabled))
	for _, e := range enabled {
		enabledKeys[specKey(e.spec)] = true
	}
	var disabled []monitorSpec
	currentKeys := make(map[string]bool, len(connected))
	for i := range connected {
		spec := connected[i].Spec
		currentKeys[specKey(spec)] = true
		if !enabledKeys[specKey(spec)] {
			disabled = append(disabled, spec)
		}
	}

	newBlock := buildConfigurationXML(enabled, disabled)

	path, err := monitorsXMLPath()
	if err != nil {
		return err
	}

	preserved := preservedConfigs(path, currentKeys)

	var b strings.Builder
	b.WriteString("<monitors version=\"2\">\n")
	b.WriteString(newBlock)
	for _, raw := range preserved {
		b.WriteString("  <configuration>")
		b.WriteString(raw)
		b.WriteString("</configuration>\n")
	}
	b.WriteString("</monitors>\n")

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return errors.Wrap(err, "creating config dir")
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return errors.Wrap(err, "writing monitors.xml")
	}
	return nil
}

// preservedConfigs returns the raw inner XML of every existing <configuration>
// that targets a *different* hardware set than the one we're rewriting. A parse
// failure (or missing file) simply yields nothing to preserve.
func preservedConfigs(path string, currentKeys map[string]bool) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var parsed monitorsFileXML
	if err := xml.Unmarshal(data, &parsed); err != nil {
		return nil
	}

	var preserved []string
	for _, cfg := range parsed.Configs {
		keys := make(map[string]bool)
		for _, s := range cfg.LogicalSpecs {
			keys[specKeyXML(s)] = true
		}
		for _, s := range cfg.DisabledSpecs {
			keys[specKeyXML(s)] = true
		}
		if sameKeySet(keys, currentKeys) {
			continue // this block is for the current hardware; we replace it
		}
		preserved = append(preserved, cfg.Inner)
	}
	return preserved
}

func sameKeySet(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}

// buildConfigurationXML renders one <configuration> block matching Mutter 46's
// on-disk format. Ottoman layouts are unrotated with one physical monitor per
// logical monitor; the scale is whatever the layout applied (1 for 100%, 2 for
// 200%, or a fractional value like 1.5).
func buildConfigurationXML(enabled []persistLogicalMonitor, disabled []monitorSpec) string {
	var b strings.Builder
	b.WriteString("  <configuration>\n")
	for _, e := range enabled {
		b.WriteString("    <logicalmonitor>\n")
		b.WriteString("      <x>" + strconv.Itoa(int(e.x)) + "</x>\n")
		b.WriteString("      <y>" + strconv.Itoa(int(e.y)) + "</y>\n")
		b.WriteString("      <scale>" + formatScaleXML(e.scale) + "</scale>\n")
		if e.primary {
			b.WriteString("      <primary>yes</primary>\n")
		}
		b.WriteString("      <monitor>\n")
		writeMonitorSpecXML(&b, e.spec, "        ")
		b.WriteString("        <mode>\n")
		b.WriteString("          <width>" + strconv.Itoa(int(e.width)) + "</width>\n")
		b.WriteString("          <height>" + strconv.Itoa(int(e.height)) + "</height>\n")
		b.WriteString("          <rate>" + strconv.FormatFloat(e.rate, 'g', -1, 64) + "</rate>\n")
		b.WriteString("        </mode>\n")
		b.WriteString("      </monitor>\n")
		b.WriteString("    </logicalmonitor>\n")
	}
	for _, spec := range disabled {
		b.WriteString("    <disabled>\n")
		b.WriteString("      <monitorspec>\n")
		writeSpecFieldsXML(&b, spec, "        ")
		b.WriteString("      </monitorspec>\n")
		b.WriteString("    </disabled>\n")
	}
	b.WriteString("  </configuration>\n")
	return b.String()
}

// formatScaleXML renders a scale the way Mutter writes it to monitors.xml: whole
// numbers with no decimal point ("2"), fractional values at full precision
// ("1.5"). An unset scale (0) defaults to 1.
func formatScaleXML(scale float64) string {
	if scale <= 0 {
		scale = 1
	}
	if scale == math.Trunc(scale) {
		return strconv.Itoa(int(scale))
	}
	return strconv.FormatFloat(scale, 'g', -1, 64)
}

func writeMonitorSpecXML(b *strings.Builder, spec monitorSpec, indent string) {
	b.WriteString(indent + "<monitorspec>\n")
	writeSpecFieldsXML(b, spec, indent+"  ")
	b.WriteString(indent + "</monitorspec>\n")
}

func writeSpecFieldsXML(b *strings.Builder, spec monitorSpec, indent string) {
	b.WriteString(indent + "<connector>" + xmlEscape(spec.Connector) + "</connector>\n")
	b.WriteString(indent + "<vendor>" + xmlEscape(spec.Vendor) + "</vendor>\n")
	b.WriteString(indent + "<product>" + xmlEscape(spec.Product) + "</product>\n")
	b.WriteString(indent + "<serial>" + xmlEscape(spec.Serial) + "</serial>\n")
}

func xmlEscape(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}
