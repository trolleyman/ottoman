package common

import (
	"runtime/debug"
	"testing"
)

func TestBuildRevision(t *testing.T) {
	for _, tc := range []struct {
		settings []debug.BuildSetting
		want     string
	}{
		{nil, "unknown"},
		{[]debug.BuildSetting{{Key: "vcs.revision", Value: "1234567890abcdef"}}, "1234567890ab"},
		{[]debug.BuildSetting{{Key: "vcs.revision", Value: "1234567890abcdef"}, {Key: "vcs.modified", Value: "true"}}, "1234567890ab-dirty"},
	} {
		if got := revisionFromSettings(tc.settings); got != tc.want {
			t.Fatalf("%q want %q", got, tc.want)
		}
	}
}
