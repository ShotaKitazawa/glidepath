// Package web renders the server-side HTML pages (SPEC.md 7: html/template,
// no frontend build).
package web

import (
	"bytes"
	"embed"
	"html/template"
	"log/slog"
	"net/http"
)

//go:embed templates/*.html
var templateFS embed.FS

// Render parses layout.html together with the named page template and
// executes it. name must be a file under templates/ that defines a
// "content" block, e.g. "inventory.html".
//
// It renders to a buffer first and only writes to w once execution has
// fully succeeded — html/template can fail partway through (e.g. a field
// referenced by the template but missing from data), and writing directly
// to w would leave a truncated 200 response with no way to recover; a
// half-rendered page with no error is worse than a 500.
func Render(w http.ResponseWriter, status int, name string, data any) {
	tmpl, err := template.New("layout.html").Funcs(funcMap).ParseFS(templateFS, "templates/layout.html", "templates/"+name)
	if err != nil {
		slog.Error("parsing template", "name", name, "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "base", data); err != nil {
		slog.Error("executing template", "name", name, "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}
