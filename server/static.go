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

// setCacheHeaders sets appropriate cache headers based on the file path.
// - Hashed assets (in /assets/): immutable, 1 year cache
// - Fonts: 1 year cache
// - Images/GIFs: 1 week cache
// - HTML/XML/TXT: no-cache (revalidate)
func setCacheHeaders(w http.ResponseWriter, filePath string) {
	switch {
	// Vite hashed assets - immutable, cache forever
	case strings.HasPrefix(filePath, "assets/"):
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")

	// Fonts - long cache
	case strings.HasSuffix(filePath, ".woff2") || strings.HasSuffix(filePath, ".woff"):
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")

	// Images and GIFs - 1 week cache
	case strings.HasSuffix(filePath, ".png") ||
		strings.HasSuffix(filePath, ".jpg") ||
		strings.HasSuffix(filePath, ".jpeg") ||
		strings.HasSuffix(filePath, ".gif") ||
		strings.HasSuffix(filePath, ".webp") ||
		strings.HasSuffix(filePath, ".svg") ||
		strings.HasSuffix(filePath, ".ico"):
		w.Header().Set("Cache-Control", "public, max-age=604800")

	// HTML, sitemap, robots - revalidate every time
	case strings.HasSuffix(filePath, ".html") ||
		strings.HasSuffix(filePath, ".xml") ||
		strings.HasSuffix(filePath, ".txt"):
		w.Header().Set("Cache-Control", "no-cache")

	// Default - short cache with revalidation
	default:
		w.Header().Set("Cache-Control", "public, max-age=3600")
	}
}

// staticFileServer serves the embedded static files from web/dist
// It handles SPA routing by returning index.html for non-file paths
func staticFileServer() http.Handler {
	// Get the dist subdirectory from the embedded filesystem
	distFS, err := fs.Sub(frontendFS, "dist")
	if err != nil {
		panic("failed to get frontend subdirectory: " + err.Error())
	}

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
			if file, err := distFS.Open(filePath); err == nil {
				file.Close()

				// Set cache headers based on file type
				setCacheHeaders(w, filePath)

				// Serve the file
				http.FileServer(http.FS(distFS)).ServeHTTP(w, r)
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

		// SPA routes should not be cached
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write(content)
	})
}
