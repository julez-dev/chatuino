package server

import (
	"io"
	"net/http"
	"net/url"
)

// DefaultHelixBaseURL is the default Twitch Helix API base URL.
const DefaultHelixBaseURL = "https://api.twitch.tv"

// handleHelixProxy returns an http.HandlerFunc that forwards requests to the Twitch Helix API.
// It rewrites /ttv/* paths to /helix/*, uses custom transport to inject auth headers,
// and copies the response back to the client.
func (a *API) handleHelixProxy() http.HandlerFunc {
	target, err := url.Parse(DefaultHelixBaseURL)
	if err != nil {
		// DefaultHelixBaseURL is hardcoded and valid, this should never happen
		panic("invalid DefaultHelixBaseURL: " + err.Error())
	}

	return a.helixProxyHandlerWithTarget(target)
}

// helixProxyHandlerWithTarget is the internal implementation that accepts a configurable target URL.
// This is used by handleHelixProxy and can be used in tests.
func (a *API) helixProxyHandlerWithTarget(target *url.URL) http.HandlerFunc {
	// Create HTTP client with custom transport that injects auth headers
	client := &http.Client{
		Transport: newHelixRetryTransport(http.DefaultTransport, a.helixTokenProvider, a.conf.ClientID),
	}

	return func(w http.ResponseWriter, r *http.Request) {
		// Only allow GET requests (all allowlisted endpoints are read-only)
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Build target URL: /ttv/chat/emotes/global -> https://api.twitch.tv/helix/chat/emotes/global
		helixPath := extractHelixPath(r.URL.Path)
		targetURL := target.ResolveReference(&url.URL{
			Path:     "/helix/" + helixPath,
			RawQuery: r.URL.RawQuery,
		})

		// Create new request to Twitch (no body needed for GET)
		req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, targetURL.String(), nil)
		if err != nil {
			logger := a.getLoggerFrom(r.Context())
			logger.Err(err).Msg("failed to create proxy request")
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		// Make request to Twitch (transport adds auth headers)
		resp, err := client.Do(req)
		if err != nil {
			logger := a.getLoggerFrom(r.Context())
			logger.Err(err).Str("url", targetURL.String()).Msg("proxy request failed")
			http.Error(w, "bad gateway", http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		// Only copy essential response headers
		// Copy Ratelimit-Reset on 429 to inform client when to retry
		if resp.StatusCode == http.StatusTooManyRequests {
			if resetHeader := resp.Header.Get("Ratelimit-Reset"); resetHeader != "" {
				w.Header().Set("Ratelimit-Reset", resetHeader)
			}
		}

		// Copy Content-Type so client knows response format
		if contentType := resp.Header.Get("Content-Type"); contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}

		// Write status code
		w.WriteHeader(resp.StatusCode)

		// Copy response body
		io.Copy(w, resp.Body)
	}
}
