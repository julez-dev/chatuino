//go:build webdist

package server

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:webdist
var frontendFS embed.FS

// staticFileServer serves the embedded static files from web/dist.
// It handles SPA routing by returning index.html for non-file paths.
func staticFileServer() http.Handler {
	distFS, err := fs.Sub(frontendFS, "webdist")
	if err != nil {
		panic("failed to get frontend subdirectory: " + err.Error())
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		urlPath := path.Clean(r.URL.Path)
		if urlPath == "" {
			urlPath = "/"
		}

		if urlPath != "/" {
			filePath := strings.TrimPrefix(urlPath, "/")

			if file, err := distFS.Open(filePath); err == nil {
				file.Close()

				setCacheHeaders(w, filePath)

				http.FileServer(http.FS(distFS)).ServeHTTP(w, r)
				return
			}
		}

		indexFile, err := distFS.Open("index.html")
		if err != nil {
			http.Error(w, "index.html not found", http.StatusInternalServerError)
			return
		}
		defer indexFile.Close()

		stat, err := indexFile.Stat()
		if err != nil {
			http.Error(w, "failed to stat index.html", http.StatusInternalServerError)
			return
		}

		content := make([]byte, stat.Size())
		if _, err := indexFile.Read(content); err != nil {
			http.Error(w, "failed to read index.html", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write(content)
	})
}

// setCacheHeaders sets appropriate cache headers based on the file path.
func setCacheHeaders(w http.ResponseWriter, filePath string) {
	switch {
	case strings.HasPrefix(filePath, "assets/"):
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")

	case strings.HasSuffix(filePath, ".woff2") || strings.HasSuffix(filePath, ".woff"):
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")

	case strings.HasSuffix(filePath, ".png") ||
		strings.HasSuffix(filePath, ".jpg") ||
		strings.HasSuffix(filePath, ".jpeg") ||
		strings.HasSuffix(filePath, ".gif") ||
		strings.HasSuffix(filePath, ".webp") ||
		strings.HasSuffix(filePath, ".svg") ||
		strings.HasSuffix(filePath, ".ico") ||
		strings.HasSuffix(filePath, ".mp4") ||
		strings.HasSuffix(filePath, ".webm"):
		w.Header().Set("Cache-Control", "public, max-age=604800")

	case strings.HasSuffix(filePath, ".html") ||
		strings.HasSuffix(filePath, ".xml") ||
		strings.HasSuffix(filePath, ".txt"):
		w.Header().Set("Cache-Control", "no-cache")

	default:
		w.Header().Set("Cache-Control", "public, max-age=3600")
	}
}
