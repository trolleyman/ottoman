package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var assets embed.FS

// DistFS returns the web client's dist directory as an fs.FS.
func DistFS() (fs.FS, error) {
	return fs.Sub(assets, "dist")
}
