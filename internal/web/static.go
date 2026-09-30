package web

import (
	"embed"
	"net/http"
)

//go:embed static/favicon.svg static/manifest.webmanifest static/sw.js
var staticFS embed.FS

// RegisterStatic wires the app's small set of static assets — no
// bundler/CDN, per SPEC.md §7. sw.js is served at the root path, not
// under /static/, so its default scope covers the whole app (a service
// worker's scope is capped to the directory it's served from).
func RegisterStatic(mux *http.ServeMux) {
	mux.HandleFunc("GET /static/favicon.svg", serveStatic("static/favicon.svg", "image/svg+xml"))
	mux.HandleFunc("GET /manifest.webmanifest", serveStatic("static/manifest.webmanifest", "application/manifest+json"))
	mux.HandleFunc("GET /sw.js", serveStatic("static/sw.js", "text/javascript"))
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
