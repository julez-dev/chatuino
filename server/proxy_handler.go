package server

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

func (a *API) handleCheckRedirectsRequest() http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawTargetURL := r.URL.Query().Get("target")
		if rawTargetURL == "" {
			http.Error(w, "Missing target URL", http.StatusBadRequest)
			return
		}

		target, err := url.Parse(rawTargetURL)
		if err != nil || target.Scheme == "" || target.Host == "" {
			http.Error(w, fmt.Sprintf("%s is invalid target URL", rawTargetURL), http.StatusBadRequest)
			return
		}

		var visited []string
		client := http.Client{
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				visited = append(visited, req.URL.String())
				return nil
			},
		}

		proxyReq, err := http.NewRequestWithContext(r.Context(), http.MethodGet, target.String(), nil)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to create proxy request: %q", err.Error()), http.StatusInternalServerError)
			return
		}

		setFakeFirefoxHeaders(proxyReq)

		resp, err := client.Do(proxyReq)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to execute proxy request: %q", err.Error()), http.StatusBadGateway)
			return
		}

		defer func() {
			_ = resp.Body.Close()
		}()

		w.Header().Set("X-Remote-Status-Code", fmt.Sprintf("%d", resp.StatusCode))
		w.Header().Set("X-Remote-Content-Type", resp.Header.Get("Content-Type"))
		w.Header().Set("X-Visited-URLs", strings.Join(visited, ","))
	})
}

func setFakeFirefoxHeaders(req *http.Request) {
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:133.0) Gecko/20100101 Firefox/133.0")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	req.Header.Set("Sec-Fetch-User", "?1")
	req.Header.Set("Priority", "u=1")
}
