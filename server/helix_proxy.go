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

			// Remove proxy-revealing headers
			req.Header.Del("X-Forwarded-For")
			req.Header.Del("X-Forwarded-Host")
			req.Header.Del("X-Forwarded-Proto")
			req.Header.Del("Forwarded")

			// Set a proper User-Agent if missing (empty User-Agent can trigger blocks)
			if req.Header.Get("User-Agent") == "" {
				req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; chatuino/1.0)")
			}
		},
		ModifyResponse: func(resp *http.Response) error {
			resp.Header.Del("Ratelimit-Limit")
			resp.Header.Del("Ratelimit-Remaining")

			// Strip rate limit headers for non-429 responses
			// Only preserve Ratelimit-Reset on 429 to inform clients when to retry
			if resp.StatusCode != http.StatusTooManyRequests {
				resp.Header.Del("Ratelimit-Reset")
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
