//go:build !webdist

package server

import "net/http"

// staticFileServer returns a handler that responds with 404 when the web
// frontend was not embedded into the binary (built without the webdist tag).
func staticFileServer() http.Handler {
	return http.NotFoundHandler()
}
