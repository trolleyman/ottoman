package agent

import (
	"strings"
	"testing"
)

func TestSetGdmAutologin(t *testing.T) {
	const user = "callum"

	// hasAutologin checks the result actually enables autologin for user in the
	// [daemon] section (single canonical copy of each key, no leftover comments).
	hasAutologin := func(t *testing.T, out string) {
		t.Helper()
		section := ""
		enable, login := 0, 0
		for _, line := range strings.Split(out, "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
				section = strings.ToLower(strings.TrimSpace(trimmed[1 : len(trimmed)-1]))
				continue
			}
			if section != "daemon" {
				continue
			}
			switch {
			case trimmed == "AutomaticLoginEnable=true":
				enable++
			case trimmed == "AutomaticLogin="+user:
				login++
			}
		}
		if enable != 1 || login != 1 {
			t.Fatalf("want exactly one AutomaticLoginEnable=true and AutomaticLogin=%s in [daemon]; got enable=%d login=%d\n---\n%s", user, enable, login, out)
		}
	}

	cases := []struct {
		name string
		in   string
	}{
		{"empty file", ""},
		{"no daemon section", "[security]\n\n[xdmcp]\n"},
		{"empty daemon section", "[daemon]\n\n[security]\n"},
		{"commented keys (ubuntu default)", "[daemon]\n# Enabling automatic login\n#  AutomaticLoginEnable = true\n#  AutomaticLogin = user1\n\n[security]\n"},
		{"already enabled for other user", "[daemon]\nAutomaticLoginEnable=true\nAutomaticLogin=someoneelse\n"},
		{"daemon is last section", "[security]\n[daemon]\nWaylandEnable=false\n"},
		{"other daemon keys preserved", "[daemon]\nWaylandEnable=false\nTimedLoginEnable=false\n\n[chooser]\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out := setGdmAutologin(c.in, user)
			hasAutologin(t, out)

			// Idempotent: applying again is a no-op.
			if again := setGdmAutologin(out, user); again != out {
				t.Fatalf("not idempotent:\n--- first ---\n%s\n--- second ---\n%s", out, again)
			}
		})
	}
}

func TestSetGdmAutologinPreservesOtherKeys(t *testing.T) {
	in := "[daemon]\nWaylandEnable=false\n\n[security]\nDisallowTCP=true\n"
	out := setGdmAutologin(in, "callum")
	for _, want := range []string{"WaylandEnable=false", "[security]", "DisallowTCP=true"} {
		if !strings.Contains(out, want) {
			t.Errorf("output dropped %q:\n%s", want, out)
		}
	}
}

func TestMatchedDaemonKey(t *testing.T) {
	cases := []struct {
		line string
		want string
	}{
		{"AutomaticLoginEnable=true", "AutomaticLoginEnable"},
		{"  AutomaticLoginEnable = true", "AutomaticLoginEnable"},
		{"#AutomaticLoginEnable=true", "AutomaticLoginEnable"},
		{"# AutomaticLogin = bob", "AutomaticLogin"},
		{"AutomaticLogin=bob", "AutomaticLogin"},
		{"WaylandEnable=false", ""},
		{"# a note mentioning AutomaticLogin somewhere", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := matchedDaemonKey(c.line); got != c.want {
			t.Errorf("matchedDaemonKey(%q) = %q, want %q", c.line, got, c.want)
		}
	}
}
