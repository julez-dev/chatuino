package mainui

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type stubVersionChecker struct {
	version string
	err     error
}

func (s stubVersionChecker) GetLatestVersion(_ context.Context) (string, error) {
	return s.version, s.err
}

func TestCheckForUpdate(t *testing.T) {
	t.Parallel()

	t.Run("skip dev build", func(t *testing.T) {
		t.Parallel()
		info, err := checkForUpdate(context.Background(), stubVersionChecker{version: "1.0.0"}, "dev")
		require.NoError(t, err)
		require.False(t, info.HasUpdate)
	})

	t.Run("skip nil checker", func(t *testing.T) {
		t.Parallel()
		info, err := checkForUpdate(context.Background(), nil, "0.6.0")
		require.NoError(t, err)
		require.False(t, info.HasUpdate)
	})

	t.Run("update available", func(t *testing.T) {
		t.Parallel()
		info, err := checkForUpdate(context.Background(), stubVersionChecker{version: "v0.7.0"}, "v0.6.0")
		require.NoError(t, err)
		require.True(t, info.HasUpdate)
		require.Equal(t, "v0.7.0", info.LatestVersion)
		require.Equal(t, "v0.6.0", info.CurrentVersion)
	})

	t.Run("update available without v prefix", func(t *testing.T) {
		t.Parallel()
		info, err := checkForUpdate(context.Background(), stubVersionChecker{version: "0.7.0"}, "0.6.0")
		require.NoError(t, err)
		require.True(t, info.HasUpdate)
		require.Equal(t, "v0.7.0", info.LatestVersion)
		require.Equal(t, "v0.6.0", info.CurrentVersion)
	})

	t.Run("no update same version", func(t *testing.T) {
		t.Parallel()
		info, err := checkForUpdate(context.Background(), stubVersionChecker{version: "v0.6.0"}, "v0.6.0")
		require.NoError(t, err)
		require.False(t, info.HasUpdate)
	})

	t.Run("no update current newer", func(t *testing.T) {
		t.Parallel()
		info, err := checkForUpdate(context.Background(), stubVersionChecker{version: "v0.5.0"}, "v0.6.0")
		require.NoError(t, err)
		require.False(t, info.HasUpdate)
		require.Equal(t, "v0.5.0", info.LatestVersion)
		require.Equal(t, "v0.6.0", info.CurrentVersion)
	})

	t.Run("server error", func(t *testing.T) {
		t.Parallel()
		_, err := checkForUpdate(context.Background(), stubVersionChecker{err: errors.New("connection refused")}, "v0.6.0")
		require.Error(t, err)
		require.Contains(t, err.Error(), "connection refused")
	})

	t.Run("invalid current version", func(t *testing.T) {
		t.Parallel()
		_, err := checkForUpdate(context.Background(), stubVersionChecker{version: "v0.7.0"}, "not-semver")
		require.Error(t, err)
		require.Contains(t, err.Error(), "not valid semver")
	})

	t.Run("invalid server version", func(t *testing.T) {
		t.Parallel()
		_, err := checkForUpdate(context.Background(), stubVersionChecker{version: "garbage"}, "v0.6.0")
		require.Error(t, err)
		require.Contains(t, err.Error(), "not valid semver")
	})

	t.Run("prerelease ordering", func(t *testing.T) {
		t.Parallel()
		info, err := checkForUpdate(context.Background(), stubVersionChecker{version: "v1.0.0"}, "v1.0.0-rc.1")
		require.NoError(t, err)
		require.True(t, info.HasUpdate)
	})
}
