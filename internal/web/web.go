// Package web serves the browser pages. They are plain files built into the program.
package web

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed static
var files embed.FS

// Handler serves the pages with headers that limit what a page may do.
func Handler() http.Handler {
	static, _ := fs.Sub(files, "static")
	fileServer := http.FileServer(http.FS(static))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cache-Control", "no-store")
		fileServer.ServeHTTP(w, r)
	})
}
