package assets

import (
	"embed"
	"io/fs"
)

// Static assets embedded at compile time
//
//go:embed static/images/* static/img/* all:static/dist frontend/core.frontend.json
var staticFiles embed.FS

// GetStaticFS returns the embedded static file system
func GetStaticFS() fs.FS {
	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		panic("failed to create static file system: " + err.Error())
	}
	return staticFS
}

// ReadStaticFile reads a file from the embedded static files
func ReadStaticFile(path string) ([]byte, error) {
	return staticFiles.ReadFile("static/" + path)
}
