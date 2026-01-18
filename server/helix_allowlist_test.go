package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtractHelixPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "global emotes",
			input:    "/ttv/chat/emotes/global",
			expected: "chat/emotes/global",
		},
		{
			name:     "channel emotes",
			input:    "/ttv/chat/emotes",
			expected: "chat/emotes",
		},
		{
			name:     "streams",
			input:    "/ttv/streams",
			expected: "streams",
		},
		{
			name:     "users",
			input:    "/ttv/users",
			expected: "users",
		},
		{
			name:     "chat settings",
			input:    "/ttv/chat/settings",
			expected: "chat/settings",
		},
		{
			name:     "global badges",
			input:    "/ttv/chat/badges/global",
			expected: "chat/badges/global",
		},
		{
			name:     "channel badges",
			input:    "/ttv/chat/badges",
			expected: "chat/badges",
		},
		{
			name:     "no prefix",
			input:    "chat/emotes",
			expected: "chat/emotes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := extractHelixPath(tt.input)
			require.Equal(t, tt.expected, got)
		})
	}
}

func TestIsPathAllowed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{"global emotes allowed", "chat/emotes/global", true},
		{"channel emotes allowed", "chat/emotes", true},
		{"streams allowed", "streams", true},
		{"users allowed", "users", true},
		{"chat settings allowed", "chat/settings", true},
		{"global badges allowed", "chat/badges/global", true},
		{"channel badges allowed", "chat/badges", true},
		{"unknown path rejected", "unknown/path", false},
		{"eventsub rejected", "eventsub/subscriptions", false},
		{"channels rejected", "channels", false},
		{"empty rejected", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := isPathAllowed(tt.path)
			require.Equal(t, tt.expected, got)
		})
	}
}

func TestHelixAllowlistMiddleware(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		path           string
		expectedStatus int
		expectHandler  bool
	}{
		{
			name:           "allowed path passes through",
			path:           "/ttv/chat/emotes/global",
			expectedStatus: http.StatusOK,
			expectHandler:  true,
		},
		{
			name:           "allowed path with query params",
			path:           "/ttv/chat/emotes",
			expectedStatus: http.StatusOK,
			expectHandler:  true,
		},
		{
			name:           "streams allowed",
			path:           "/ttv/streams",
			expectedStatus: http.StatusOK,
			expectHandler:  true,
		},
		{
			name:           "users allowed",
			path:           "/ttv/users",
			expectedStatus: http.StatusOK,
			expectHandler:  true,
		},
		{
			name:           "non-allowed path returns 403",
			path:           "/ttv/eventsub/subscriptions",
			expectedStatus: http.StatusForbidden,
			expectHandler:  false,
		},
		{
			name:           "unknown path returns 403",
			path:           "/ttv/unknown/endpoint",
			expectedStatus: http.StatusForbidden,
			expectHandler:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			handlerCalled := false
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				handlerCalled = true
				w.WriteHeader(http.StatusOK)
			})

			middleware := HelixAllowlistMiddleware(handler)
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()

			middleware.ServeHTTP(rec, req)

			require.Equal(t, tt.expectedStatus, rec.Code)
			require.Equal(t, tt.expectHandler, handlerCalled)
		})
	}
}
