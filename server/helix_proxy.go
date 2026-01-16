package server

import (
	"net/http"
	"net/http/httputil"
	"net/url"
)

// DefaultHelixBaseURL is the default Twitch Helix API base URL.
const DefaultHelixBaseURL = "https://api.twitch.tv"

// handleHelixProxy returns an http.HandlerFunc that proxies requests to the Twitch Helix API.
// It rewrites /ttv/* paths to /helix/*, injects auth headers, and passes through responses unchanged.
func (a *API) handleHelixProxy() http.HandlerFunc {
	target, err := url.Parse(DefaultHelixBaseURL)
	if err != nil {
		// DefaultHelixBaseURL is hardcoded and valid, this should never happen
		panic("invalid DefaultHelixBaseURL: " + err.Error())
	}
	proxy := a.helixProxyHandlerWithTarget(target)
	return proxy.ServeHTTP
}

// helixProxyHandlerWithTarget is the internal implementation that accepts a configurable target URL.
// This is used by HelixProxyHandler and can be used in tests.
func (a *API) helixProxyHandlerWithTarget(target *url.URL) http.Handler {
	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			// Set target URL
			req.URL.Scheme = target.Scheme
			req.URL.Host = target.Host

			// Rewrite path: /ttv/chat/emotes -> /helix/chat/emotes
			helixPath := extractHelixPath(req.URL.Path)
			req.URL.Path = "/helix/" + helixPath

			// Remove X-Forwarded-For header
			req.Header.Del("X-Forwarded-For")

			// Set required proxy headers
			if _, ok := req.Header["User-Agent"]; !ok {
				// Set a default user agent if not present
				req.Header.Set("User-Agent", "")
			}
		},
		ModifyResponse: func(resp *http.Response) error {
			// Strip rate limit headers for non-429 responses
			// Only preserve Ratelimit-Reset on 429 to inform clients when to retry
			if resp.StatusCode != http.StatusTooManyRequests {
				resp.Header.Del("Ratelimit-Limit")
				resp.Header.Del("Ratelimit-Remaining")
				resp.Header.Del("Ratelimit-Reset")
			} else {
				// For 429, keep only Ratelimit-Reset
				resp.Header.Del("Ratelimit-Limit")
				resp.Header.Del("Ratelimit-Remaining")
			}
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			logger := a.getLoggerFrom(r.Context())
			logger.Err(err).Str("url", r.URL.String()).Msg("proxy error")
			w.WriteHeader(http.StatusBadGateway)
		},
		Transport: newHelixRetryTransport(http.DefaultTransport, a.helixTokenProvider, a.conf.ClientID),
	}

	return proxy
}
