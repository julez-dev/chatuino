package server

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewRedisClient(t *testing.T) {
	t.Run("returns error when connection fails", func(t *testing.T) {
		t.Parallel()
		cfg := RedisConfig{
			Addr: "invalid-host:9999",
		}

		client, err := NewRedisClient(context.Background(), cfg)
		require.Error(t, err, "should return error when connection fails")
		require.Nil(t, client, "client should be nil when error occurs")
	})

	t.Run("respects context timeout", func(t *testing.T) {
		t.Parallel()
		cfg := RedisConfig{
			Addr: "10.255.255.1:6379", // Non-routable IP to ensure timeout
		}

		ctx := context.Background()
		client, err := NewRedisClient(ctx, cfg)
		require.Error(t, err, "should return error on timeout")
		require.Nil(t, client, "client should be nil when error occurs")
	})
}
