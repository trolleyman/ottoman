//go:build linux

package input

import (
	"context"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

var xkbSource = regexp.MustCompile(`\('xkb', '([^']+)'\)`)

// GNOME keeps the last interactively selected source first in mru-sources.
// Per-window layouts and non-US/GB maps need a compositor text provider.
// Read the remote session's
// settings, never the browser locale. A timeout bounds typing latency if its
// settings service is unavailable. Cache briefly so rapid taps don't fork a
// process per character; switching layouts is picked up on the next second.
func readTextLayout() string {
	for _, key := range []string{"mru-sources", "sources"} {
		ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
		output, err := exec.CommandContext(ctx, "gsettings", "get", "org.gnome.desktop.input-sources", key).Output()
		cancel()
		if err == nil {
			if match := xkbSource.FindStringSubmatch(string(output)); len(match) > 1 {
				return match[1]
			}
		}
	}
	if layout := os.Getenv("XKB_DEFAULT_LAYOUT"); layout != "" {
		return strings.Split(layout, ",")[0]
	}
	return "us"
}

func textKey(r rune, layout string) (uint16, []string, bool) {
	shifted := r >= 'A' && r <= 'Z' || strings.ContainsRune("!@#$%^&*()_+{}|:\"~<>?", r)
	code, ok := charToKeyCode(r)
	if layout == "gb" || strings.HasPrefix(layout, "gb+") {
		switch r {
		case '"':
			code, shifted, ok = 3, true, true
		case '@':
			code, shifted, ok = 40, true, true
		case '#':
			code, shifted, ok = 43, false, true
		case '~':
			code, shifted, ok = 43, true, true
		case '\\':
			code, shifted, ok = 86, false, true
		case '|':
			code, shifted, ok = 86, true, true
		case '£':
			code, shifted, ok = 4, true, true
		case '¬':
			code, shifted, ok = 41, true, true
		}
	}
	var modifiers []string
	if shifted {
		modifiers = []string{"shift"}
	}
	return code, modifiers, ok
}

func (k *UinputKeyboard) TypeText(text string) error {
	k.textMu.Lock()
	defer k.textMu.Unlock()
	if time.Since(k.layoutRead) > time.Second {
		k.layout = readTextLayout()
		k.layoutRead = time.Now()
	}
	for _, r := range text {
		code, mods, ok := textKey(r, k.layout)
		if !ok {
			continue
		}
		if err := k.emitCode(code, mods, 1); err != nil {
			return err
		}
		if err := k.emitCode(code, mods, 0); err != nil {
			return err
		}
	}
	return nil
}
