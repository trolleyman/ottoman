package common

import "runtime/debug"

// BuildRevision identifies the source embedded in this binary, including local
// changes. Go records it for normal and cross-compiled builds automatically.
func BuildRevision() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	return revisionFromSettings(info.Settings)
}
func revisionFromSettings(settings []debug.BuildSetting) string {
	revision, dirty := "", false
	for _, setting := range settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			dirty = setting.Value == "true"
		}
	}
	if revision == "" {
		return "unknown"
	}
	if len(revision) > 12 {
		revision = revision[:12]
	}
	if dirty {
		revision += "-dirty"
	}
	return revision
}
