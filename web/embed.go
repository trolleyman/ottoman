package web

import (
	"embed"
	"io/fs"
)

//go:embed all:client/dist
var assets embed.FS

// ClientDistFS returns the web client's dist directory as an fs.FS.
func ClientDistFS() (fs.FS, error) {
	return fs.Sub(assets, "client/dist")
}
