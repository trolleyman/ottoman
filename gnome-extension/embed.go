// Package gnomeext embeds the Ottoman GNOME Shell Quick Settings extension so
// the agent binary can install it during `ottoman agent install` without
// needing the repository checkout on the target machine.
package gnomeext

import (
	"embed"
	"io/fs"
)

// UUID is the extension's uuid (matches metadata.json). It is also the
// directory name the extension must live in under the user's extensions dir.
const UUID = "ottoman@trolleyman"

//go:embed metadata.json extension.js
var files embed.FS

// Files returns the embedded extension files (metadata.json, extension.js) as
// an fs.FS whose entries sit at the root.
func Files() fs.FS {
	return files
}
