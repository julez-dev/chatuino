package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHandleGetVersion(t *testing.T) {
	t.Parallel()

	t.Run("returns configured version", func(t *testing.T) {
		t.Parallel()

		api := createTestAPI(t)
		api.conf.Version = "0.7.0"
		handler := api.handleGetVersion()

		req := httptest.NewRequest(http.MethodGet, "/version", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		require.Equal(t, "application/json", rec.Header().Get("Content-Type"))

		var resp VersionResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		require.Equal(t, "0.7.0", resp.Version)
	})

	t.Run("returns dev when version unset", func(t *testing.T) {
		t.Parallel()

		api := createTestAPI(t)
		handler := api.handleGetVersion()

		req := httptest.NewRequest(http.MethodGet, "/version", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)

		var resp VersionResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		require.Empty(t, resp.Version)
	})
}
