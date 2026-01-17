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
		// Build target URL: /ttv/chat/emotes/global -> https://api.twitch.tv/helix/chat/emotes/global
		helixPath := extractHelixPath(r.URL.Path)
		targetURL := target.ResolveReference(&url.URL{
			Path:     "/helix/" + helixPath,
			RawQuery: r.URL.RawQuery,
		})

		// Create new request to Twitch
		req, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL.String(), r.Body)
		if err != nil {
			logger := a.getLoggerFrom(r.Context())
			logger.Err(err).Msg("failed to create proxy request")
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		// Copy safe headers from original request (skip Host, Connection, etc.)
		// Transport will add Authorization and Client-Id
		copyHeaders(req.Header, r.Header)

		// Make request to Twitch (transport adds auth headers)
		resp, err := client.Do(req)
		if err != nil {
			logger := a.getLoggerFrom(r.Context())
			logger.Err(err).Str("url", targetURL.String()).Msg("proxy request failed")
			http.Error(w, "bad gateway", http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		// Filter response headers before sending to client
		copyResponseHeaders(w.Header(), resp.Header, resp.StatusCode)

		// Write status code
		w.WriteHeader(resp.StatusCode)

		// Copy response body
		io.Copy(w, resp.Body)
	}
}

// copyHeaders copies HTTP headers from src to dst, excluding hop-by-hop headers
func copyHeaders(dst, src http.Header) {
	// Headers to skip (hop-by-hop headers and headers we'll set ourselves)
	skipHeaders := map[string]bool{
		"Connection":          true,
		"Keep-Alive":          true,
		"Proxy-Authenticate":  true,
		"Proxy-Authorization": true,
		"Te":                  true,
		"Trailer":             true,
		"Transfer-Encoding":   true,
		"Upgrade":             true,
		"Authorization":       true, // Set by transport
		"Client-Id":           true, // Set by transport
		"Host":                true, // Set by http.Client
	}

	for key, values := range src {
		if skipHeaders[key] {
			continue
		}
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

// copyResponseHeaders copies response headers, filtering out rate limit headers except on 429
func copyResponseHeaders(dst, src http.Header, statusCode int) {
	for key, values := range src {
		// Filter rate limit headers
		if key == "Ratelimit-Limit" || key == "Ratelimit-Remaining" {
			continue
		}
		if key == "Ratelimit-Reset" && statusCode != http.StatusTooManyRequests {
			continue
		}

		for _, value := range values {
			dst.Add(key, value)
		}
	}
}
