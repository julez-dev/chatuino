package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func (a *API) handleCheckRedirectsRequest() http.HandlerFunc {
	dialFunc := http.DefaultTransport.(*http.Transport).DialContext
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, _, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}

			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, err
			}

			for _, ipAddr := range ips {
				if isBlockedIP(ipAddr.IP) {
					return nil, fmt.Errorf("connection to private/local IP address blocked: %s resolves to %s", host, ipAddr.IP)
				}
			}

			return dialFunc(ctx, network, addr)
		},
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger := a.getLoggerFrom(r.Context())

		rawTargetURL := r.URL.Query().Get("target")
		if rawTargetURL == "" {
			http.Error(w, "Missing target URL", http.StatusBadRequest)
			return
		}

		target, err := url.Parse(rawTargetURL)
		if err != nil {
			http.Error(w, fmt.Sprintf("%s is invalid target URL", rawTargetURL), http.StatusBadRequest)
			return
		}

		if err := validateURLSecurity(target); err != nil {
			logger.Err(err).Str("target", target.String()).Msg("got link check request for suspicious URL")
			http.Error(w, fmt.Sprintf("%s is invalid target URL: %s", rawTargetURL, err), http.StatusBadRequest)
			return
		}

		var visited []string
		client := http.Client{
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if err := validateURLSecurity(req.URL); err != nil {
					logger.Err(err).Str("target", req.URL.String()).Msg("got link check request for suspicious URL")
					return err
				}

				visited = append(visited, req.URL.String())

				if len(via) > 10 {
					return fmt.Errorf("too many redirects")
				}

				return nil
			},
			Transport: transport,
		}

		// prevent DoS attacks
		ctx, cancel := context.WithTimeout(r.Context(), time.Second*10)
		defer cancel()

		proxyReq, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to create proxy request: %q", err.Error()), http.StatusInternalServerError)
			return
		}

		setFakeFirefoxHeaders(proxyReq)

		resp, err := client.Do(proxyReq)
		if err != nil {
			logger.Err(err).Str("og_target", target.String()).Strs("visited", visited).Msg("failed link checker proxy request")
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

// isBlockedIP checks if an IP address is in a blocked range
func isBlockedIP(ip net.IP) bool {
	// Loopback (127.0.0.0/8, ::1)
	if ip.IsLoopback() {
		return true
	}

	// Private networks (10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16, fc00::/7)
	if ip.IsPrivate() {
		return true
	}

	// Link-local (169.254.0.0/16, fe80::/10)
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}

	// Multicast
	if ip.IsMulticast() {
		return true
	}

	// Unspecified (0.0.0.0, ::)
	if ip.IsUnspecified() {
		return true
	}

	if ip4 := ip.To4(); ip4 != nil {
		// 0.0.0.0/8 (current network)
		if ip4[0] == 0 {
			return true
		}
		// 255.255.255.255/32 (broadcast)
		if ip4[0] == 255 && ip4[1] == 255 && ip4[2] == 255 && ip4[3] == 255 {
			return true
		}
		// cloud metadata endpoint
		if ip4[0] == 169 && ip4[1] == 254 && ip4[2] == 169 && ip4[3] == 254 {
			return true
		}
	}

	return false
}

// validateURLSecurity checks if a URL is safe to access
func validateURLSecurity(u *url.URL) error {
	if u.Host == "" {
		return fmt.Errorf("empty host")
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("unsupported scheme")
	}

	hostname := u.Hostname()

	if strings.HasSuffix(strings.ToLower(hostname), ".localhost") {
		return fmt.Errorf("localhost subdomain not allowed")
	}

	if ip := net.ParseIP(hostname); ip != nil {
		if isBlockedIP(ip) {
			return fmt.Errorf("access to this IP range is not allowed")
		}
	}

	return nil
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
