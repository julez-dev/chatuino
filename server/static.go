package server

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var frontendFS embed.FS

// staticFileServer serves the embedded static files from web/dist
// It handles SPA routing by returning index.html for non-file paths
func staticFileServer() http.Handler {
	// Get the dist subdirectory from the embedded filesystem
	distFS, err := fs.Sub(frontendFS, "dist")
	if err != nil {
		panic("failed to get frontend subdirectory: " + err.Error())
	}

	fileServer := http.FileServer(http.FS(distFS))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Clean the path
		urlPath := path.Clean(r.URL.Path)
		if urlPath == "" {
			urlPath = "/"
		}

		// Try to open the file to check if it exists
		if urlPath != "/" {
			// Remove leading slash for fs.Open
			filePath := strings.TrimPrefix(urlPath, "/")

			// Check if the file exists in the embedded filesystem
			if _, err := fs.Stat(distFS, filePath); err == nil {
				// File exists, serve it directly
				fileServer.ServeHTTP(w, r)
				return
			}
		}

		// For SPA routes (non-existent files), serve index.html
		// This handles routes like /docs/features, /docs/settings, etc.
		indexFile, err := distFS.Open("index.html")
		if err != nil {
			http.Error(w, "index.html not found", http.StatusInternalServerError)
			return
		}
		defer indexFile.Close()

		// Get file info for content length
		stat, err := indexFile.Stat()
		if err != nil {
			http.Error(w, "failed to stat index.html", http.StatusInternalServerError)
			return
		}

		// Read the file content
		content := make([]byte, stat.Size())
		if _, err := indexFile.Read(content); err != nil {
			http.Error(w, "failed to read index.html", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write(content)
	})
}
