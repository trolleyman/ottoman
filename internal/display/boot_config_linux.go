//go:build linux

package display

import "log"

// Keeping the TV out of the boot configuration.
//
// Mutter restores monitors.xml before any agent exists, so whatever is in there
// decides what the first frame lands on. A TV in standby still holds its HDMI
// link up, so a persisted layout naming it comes back on a dark panel and the
// only remedy is to notice and switch. Excluding the TV from what gets written
// makes the desktop come up on the monitors every time; moving to the TV stays
// a live choice (applied with the TEMPORARY method), and a choice made while
// the machine is off is queued by the controller instead.

// ExcludeFromBootConfig implements BootConfigManager.
func (m *MutterManager) ExcludeFromBootConfig(exclude func(edid string) bool) {
	m.excludeFromBoot = exclude
}

// ReconcileBootConfig implements BootConfigManager: it re-persists the live
// arrangement minus the excluded monitors. Cheap and idempotent - the write is
// skipped when the resulting file would be identical, which is the normal case.
func (m *MutterManager) ReconcileBootConfig() error {
	if m.excludeFromBoot == nil {
		return nil // nothing to exclude, so nothing to repair
	}
	_, monitors, logical, _, err := m.getCurrentState()
	if err != nil {
		return err
	}
	return m.persistBootConfig(liveBootMonitors(monitors, logical), monitors)
}

// liveBootMonitors describes the arrangement currently on screen in the form
// monitors.xml wants, so it can be re-persisted without applying anything.
//
// No coordinate conversion: getCurrentState reports positions in the current
// layout mode's space, and that is the space monitors.xml uses too.
func liveBootMonitors(monitors []mutterMonitor, logical []mutterLogicalMonitor) []persistLogicalMonitor {
	byConnector := make(map[string]*mutterMonitor, len(monitors))
	for i := range monitors {
		byConnector[monitors[i].Spec.Connector] = &monitors[i]
	}

	var out []persistLogicalMonitor
	for _, lm := range logical {
		for _, spec := range lm.Monitors {
			mon, ok := byConnector[spec.Connector]
			if !ok {
				continue
			}
			cur := currentMode(*mon)
			if cur == nil {
				continue
			}
			out = append(out, persistLogicalMonitor{
				spec:    mon.Spec,
				x:       lm.X,
				y:       lm.Y,
				width:   cur.Width,
				height:  cur.Height,
				rate:    cur.RefreshRate,
				scale:   lm.Scale,
				primary: lm.Primary,
			})
		}
	}
	return out
}

// persistBootConfig writes the boot configuration for an arrangement, dropping
// the excluded monitors so the machine never comes back up on one.
//
// Dropping a monitor can leave the survivors at an offset, or with no primary
// if the excluded one held it, so both are repaired - the same repairs the
// agent's TV-off recovery makes when it synthesises a layout. A layout whose
// only screen is excluded leaves the previous boot configuration alone: there
// would be nothing left to come up on, and the last good one is a better guess
// than an empty screen.
func (m *MutterManager) persistBootConfig(enabled []persistLogicalMonitor, connected []mutterMonitor) error {
	keep := enabled
	if m.excludeFromBoot != nil {
		keep = nil
		for _, e := range enabled {
			if m.excludeFromBoot(monitorEDID(e.spec)) {
				continue
			}
			keep = append(keep, e)
		}
	}
	if len(keep) == 0 {
		if len(enabled) > 0 {
			log.Printf("Not persisting this layout as the boot configuration: every monitor in it is excluded")
		}
		return nil
	}
	if len(keep) != len(enabled) {
		keep = normaliseBootMonitors(keep)
	}
	return writeMonitorsXML(keep, connected)
}

// normaliseBootMonitors makes a set of monitors stand on its own after others
// were dropped: the origin moves back to (0,0) and, if the primary went with
// the dropped ones, the leftmost survivor takes over. Mutter rejects a
// configuration whose logical monitors start at an offset, and one with no
// primary leaves the shell guessing.
func normaliseBootMonitors(mons []persistLogicalMonitor) []persistLogicalMonitor {
	if len(mons) == 0 {
		return mons
	}
	out := make([]persistLogicalMonitor, len(mons))
	copy(out, mons)

	minX, minY := out[0].x, out[0].y
	hasPrimary := false
	lead := 0
	for i, m := range out {
		if m.x < minX {
			minX = m.x
		}
		if m.y < minY {
			minY = m.y
		}
		if m.primary {
			hasPrimary = true
		}
		if m.x < out[lead].x {
			lead = i
		}
	}
	for i := range out {
		out[i].x -= minX
		out[i].y -= minY
	}
	if !hasPrimary {
		out[lead].primary = true
	}
	return out
}
