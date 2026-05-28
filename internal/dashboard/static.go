package dashboard

import (
	"io/fs"
	"net/http"
	"strings"
)

// spaHandler serves static files from fsys and falls back to index.html for any
// non-API, non-asset path. This makes the dashboard a true SPA without bundlers.
func spaHandler(fsys http.FileSystem) http.Handler {
	fileServer := http.FileServer(fsys)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		f, err := fsys.Open(path)
		if err != nil {
			// Fall through to index.html for unknown paths (SPA routing).
			if isAssetPath(path) {
				http.NotFound(w, r)
				return
			}
			r2 := r.Clone(r.Context())
			r2.URL.Path = "/"
			fileServer.ServeHTTP(w, r2)
			return
		}
		_ = f.Close()
		fileServer.ServeHTTP(w, r)
	})
}

// isAssetPath identifies paths that should 404 rather than fall back to index.
func isAssetPath(p string) bool {
	for _, ext := range []string{".js", ".css", ".map", ".ico", ".png", ".jpg", ".svg", ".json", ".woff", ".woff2"} {
		if strings.HasSuffix(p, ext) {
			return true
		}
	}
	return false
}

// withCORS adds permissive CORS for local development conveniences. In a real
// product these would be tighter, but the dashboard binds to localhost by default.
func withCORS(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		h.ServeHTTP(w, r)
	})
}

// Ensure fs is referenced (silences unused-import in some IDE configs).
var _ = fs.ValidPath
