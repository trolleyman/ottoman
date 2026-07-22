package agent

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/trolleyman/ottoman/internal/api"
	"github.com/trolleyman/ottoman/internal/store"
)

// tvProbeTimeout bounds how long startup waits on a single TV power probe. A
// dark TV that keeps its HDMI link up (so the compositor still lists it as a
// connected monitor) is only detectable over the network, and a powered-off set
// answers slowly or not at all. This cap keeps the one-time startup check from
// stalling boot for long — the probe never runs on the request path.
const tvProbeTimeout = 6 * time.Second

// correctStartupDisplay steers the user session's display away from a
// powered-off TV at startup. When the machine was last used on the TV and is now
// booted with it switched off, the session comes up on the TV (which keeps its
// HDMI link up in standby, so the compositor still drives it) and part or all of
// the desktop lands on a dark panel. This detects that and switches to a layout
// that only uses the monitors that are actually lit. Best-effort and safe: if
// nothing is correctable it leaves the display untouched.
func (a *Agent) correctStartupDisplay() {
	monitors, err := a.displayMgr.ListMonitors()
	if err != nil {
		log.Printf("Startup display check: failed to list monitors: %v", err)
		return
	}
	off := a.offScreenTVs(monitors)
	if len(off) == 0 {
		return // every connected panel is on; nothing to correct
	}
	a.applyOffTVRecovery(monitors, off)
}

// offScreenTVs returns the EDIDs of active TV-backed monitors whose panel is
// currently off. Only TV-backed monitors are probed, so a machine with no TVs
// pays no network cost; each probe is bounded by tvProbeTimeout. An unreachable
// TV is treated as off — at startup that's the safe assumption (better to fall
// back to the lit monitors than strand the desktop on a panel we can't confirm
// is on).
func (a *Agent) offScreenTVs(monitors []api.Monitor) []string {
	if a.tv == nil || a.control == nil {
		return nil
	}
	var off []string
	for _, m := range monitors {
		if m.Active == nil || m.Edid == "" {
			continue
		}
		if a.control.backendFor(m.Edid) != store.BackendTV {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), tvProbeTimeout)
		on := a.tv.Reachable(ctx, m.Edid)
		cancel()
		if !on {
			off = append(off, m.Edid)
		}
	}
	return off
}

// startupCandidate is one layout the recovery may apply, in preference order.
type startupCandidate struct {
	layout api.Layout
	desc   string
	record bool // persist as the current layout on success (skip for synthesized)
}

// applyOffTVRecovery switches to a layout that avoids the powered-off TVs in
// off. It tries, in order: the saved layout matching exactly the powered-on
// monitors, then the default layout "1", then a synthesized copy of the current
// arrangement with the dark TVs dropped. The first that applies cleanly wins. It
// gives up (leaving the display as-is) only when the sole connected screen is a
// powered-off TV, since there's then nothing to show. Returns whether a layout
// was applied.
func (a *Agent) applyOffTVRecovery(monitors []api.Monitor, off []string) bool {
	offSet := make(map[string]bool, len(off))
	for _, e := range off {
		offSet[e] = true
	}

	var onEdids []string
	for _, m := range monitors {
		if m.Active == nil || m.Edid == "" || offSet[m.Edid] {
			continue
		}
		onEdids = append(onEdids, m.Edid)
	}

	log.Printf("Startup display check: %s powered off; steering the display onto the %d lit monitor(s)",
		tvList(monitors, off), len(onEdids))

	if len(onEdids) == 0 {
		log.Printf("Startup display check: the only connected screen is a powered-off TV; leaving display as-is")
		return false
	}

	for _, cand := range a.recoveryCandidates(monitors, onEdids, offSet) {
		if err := a.displayMgr.ApplyLayoutConfig(cand.layout); err != nil {
			log.Printf("Startup display check: %s did not apply: %v", cand.desc, err)
			continue
		}
		log.Printf("Startup display check: applied %s", cand.desc)
		a.currentLayout = cand.layout.Id
		if cand.record && cand.layout.Id != "" {
			a.recordCurrentLayout(cand.layout.Id)
		}
		return true
	}
	log.Printf("Startup display check: no usable layout for the powered-on monitors; leaving display as-is")
	return false
}

// recoveryCandidates builds the ordered list of layouts applyOffTVRecovery will
// try for the powered-on monitors.
func (a *Agent) recoveryCandidates(monitors []api.Monitor, onEdids []string, offSet map[string]bool) []startupCandidate {
	var cands []startupCandidate
	if id, ok := a.layouts.MatchBySet(onEdids); ok {
		if l, ok := a.layouts.Get(id); ok {
			cands = append(cands, startupCandidate{layout: l, desc: fmt.Sprintf("layout %q (matches the lit monitors)", layoutLabel(l)), record: true})
		}
	}
	if l, ok := a.layouts.Get("1"); ok {
		cands = append(cands, startupCandidate{layout: l, desc: fmt.Sprintf("default layout %q", layoutLabel(l)), record: true})
	}
	if synth, ok := synthesizeWithout(monitors, offSet); ok {
		cands = append(cands, startupCandidate{layout: synth, desc: "a synthesized TV-free layout", record: false})
	}
	return cands
}

// synthesizeWithout builds an ad-hoc layout from the currently-active monitors,
// dropping the ones in offSet. If the primary was one of the dropped monitors it
// promotes the leftmost survivor, and it normalises the origin to (0,0) so the
// remaining monitors don't start at an offset the compositor would reject. The
// result is unsaved (empty ID) — a last resort when there's no matching or
// default layout to fall back to. ok is false if nothing would remain.
func synthesizeWithout(monitors []api.Monitor, offSet map[string]bool) (api.Layout, bool) {
	var lms []api.LayoutMonitor
	hasPrimary := false
	for _, m := range monitors {
		if m.Active == nil || offSet[m.Edid] {
			continue
		}
		lm := api.LayoutMonitor{
			Name:        m.Name,
			Edid:        m.Edid,
			Port:        m.Port,
			Width:       m.Active.Width,
			Height:      m.Active.Height,
			RefreshRate: m.Active.RefreshRate,
			PositionX:   m.Active.PositionX,
			PositionY:   m.Active.PositionY,
			Primary:     m.Active.Primary,
			Scale:       m.Active.Scale,
		}
		if lm.Primary {
			hasPrimary = true
		}
		lms = append(lms, lm)
	}
	if len(lms) == 0 {
		return api.Layout{}, false
	}

	if !hasPrimary {
		lead := 0
		for i := range lms {
			if lms[i].PositionX < lms[lead].PositionX {
				lead = i
			}
		}
		lms[lead].Primary = true
	}

	minX, minY := lms[0].PositionX, lms[0].PositionY
	for _, lm := range lms {
		if lm.PositionX < minX {
			minX = lm.PositionX
		}
		if lm.PositionY < minY {
			minY = lm.PositionY
		}
	}
	for i := range lms {
		lms[i].PositionX -= minX
		lms[i].PositionY -= minY
	}

	return api.Layout{Name: "auto (TV off)", Monitors: lms}, true
}

// layoutLabel returns a human-friendly name for a layout for log lines.
func layoutLabel(l api.Layout) string {
	if l.Name != "" {
		return l.Name
	}
	return l.Id
}

// tvList formats the powered-off TVs for a log line, naming them where possible.
func tvList(monitors []api.Monitor, off []string) string {
	byEDID := make(map[string]string, len(monitors))
	for _, m := range monitors {
		byEDID[m.Edid] = m.Name
	}
	names := make([]string, 0, len(off))
	for _, e := range off {
		if n := byEDID[e]; n != "" {
			names = append(names, n)
		} else {
			names = append(names, e)
		}
	}
	if len(names) == 1 {
		return "TV " + names[0]
	}
	return fmt.Sprintf("%d TVs (%s)", len(names), strings.Join(names, ", "))
}
