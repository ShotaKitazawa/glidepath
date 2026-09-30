package web

import (
	"embed"
	"net/http"
)

//go:embed static/favicon.svg
var staticFS embed.FS

// RegisterStatic wires the app's small set of static assets — no
// bundler/CDN, per SPEC.md §7.
func RegisterStatic(mux *http.ServeMux) {
	mux.HandleFunc("GET /static/favicon.svg", serveStatic("static/favicon.svg", "image/svg+xml"))
}

func serveStatic(path, contentType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		b, err := staticFS.ReadFile(path)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", contentType)
		_, _ = w.Write(b)
	}
}
