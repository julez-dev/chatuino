package server

import (
	"net/http"
	"strings"
)

// allowedHelixPaths defines the Twitch Helix API paths that can be proxied.
// Paths are matched without the /ttv prefix and without query parameters.
// These are read-only endpoints compatible with app access tokens.
var allowedHelixPaths = map[string]struct{}{
	"chat/emotes/global": {},
	"chat/emotes":        {},
	"streams":            {},
	"users":              {},
	"chat/settings":      {},
	"chat/badges/global": {},
	"chat/badges":        {},
}

// HelixAllowlistMiddleware returns a middleware that checks if the request path
// (after stripping the /ttv prefix) is in the allowlist. Returns 403 Forbidden
// for non-allowlisted paths.
func HelixAllowlistMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := extractHelixPath(r.URL.Path)

		if !isPathAllowed(path) {
			w.WriteHeader(http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// extractHelixPath strips the /ttv/ prefix from the path.
// Input: /ttv/chat/emotes/global -> Output: chat/emotes/global
// Input: /ttv/chat/emotes -> Output: chat/emotes
func extractHelixPath(path string) string {
	path = strings.TrimPrefix(path, "/ttv/")
	path = strings.TrimPrefix(path, "/ttv")
	return path
}

// isPathAllowed checks if the given path is in the allowlist.
func isPathAllowed(path string) bool {
	_, ok := allowedHelixPaths[path]
	return ok
}
