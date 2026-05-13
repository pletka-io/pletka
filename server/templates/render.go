package templates

import (
	"embed"
	"html/template"
	"io"
)

// PageData is the only HTML template input the server needs. All page-specific
// UI state lives in SchemaJSON and is interpreted by the renderer.
type PageData struct {
	Lang       string
	Title      string
	SchemaJSON string
	CSSFile    string
	JSFile     string
}

//go:embed page.gohtml
var templateFS embed.FS

var pageTemplate = template.Must(template.ParseFS(templateFS, "page.gohtml"))

// RenderPage writes the renderer shell.
func RenderPage(w io.Writer, data PageData) error {
	return pageTemplate.ExecuteTemplate(w, "page.gohtml", data)
}
