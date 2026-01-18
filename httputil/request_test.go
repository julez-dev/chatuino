package httputil

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCloneRequest(t *testing.T) {
	t.Run("clones request without body", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, "http://example.com/test", nil)
		require.NoError(t, err)

		clone, err := CloneRequest(req)
		require.NoError(t, err)
		require.NotNil(t, clone)
		require.Equal(t, req.URL.String(), clone.URL.String())
		require.Nil(t, clone.Body)
	})

	t.Run("clones request with GetBody set", func(t *testing.T) {
		bodyContent := "test body content"
		req, err := http.NewRequest(http.MethodPost, "http://example.com/test", strings.NewReader(bodyContent))
		require.NoError(t, err)
		require.NotNil(t, req.GetBody, "http.NewRequest should set GetBody")

		clone, err := CloneRequest(req)
		require.NoError(t, err)

		// Read clone body
		cloneBody, err := io.ReadAll(clone.Body)
		require.NoError(t, err)
		require.Equal(t, bodyContent, string(cloneBody))

		// Original body should still be readable (not consumed)
		origBody, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		require.Equal(t, bodyContent, string(origBody))
	})

	t.Run("clones request without GetBody (fallback)", func(t *testing.T) {
		bodyContent := "test body content"
		req, err := http.NewRequest(http.MethodPost, "http://example.com/test", nil)
		require.NoError(t, err)

		// Manually set body without GetBody to test fallback
		req.Body = io.NopCloser(bytes.NewReader([]byte(bodyContent)))
		req.GetBody = nil

		clone, err := CloneRequest(req)
		require.NoError(t, err)

		// Read clone body
		cloneBody, err := io.ReadAll(clone.Body)
		require.NoError(t, err)
		require.Equal(t, bodyContent, string(cloneBody))

		// Original body should be restored and readable
		origBody, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		require.Equal(t, bodyContent, string(origBody))
	})

	t.Run("can clone multiple times", func(t *testing.T) {
		bodyContent := "test body content"
		req, err := http.NewRequest(http.MethodPost, "http://example.com/test", strings.NewReader(bodyContent))
		require.NoError(t, err)

		// First clone
		clone1, err := CloneRequest(req)
		require.NoError(t, err)

		// Second clone from original
		clone2, err := CloneRequest(req)
		require.NoError(t, err)

		// Both clones should have the same body
		body1, err := io.ReadAll(clone1.Body)
		require.NoError(t, err)
		require.Equal(t, bodyContent, string(body1))

		body2, err := io.ReadAll(clone2.Body)
		require.NoError(t, err)
		require.Equal(t, bodyContent, string(body2))
	})

	t.Run("preserves headers and context", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodPost, "http://example.com/test", strings.NewReader("body"))
		require.NoError(t, err)

		req.Header.Set("X-Custom-Header", "custom-value")
		req.Header.Set("Authorization", "Bearer token")

		clone, err := CloneRequest(req)
		require.NoError(t, err)

		require.Equal(t, "custom-value", clone.Header.Get("X-Custom-Header"))
		require.Equal(t, "Bearer token", clone.Header.Get("Authorization"))
		require.Equal(t, req.Context(), clone.Context())
	})
}
