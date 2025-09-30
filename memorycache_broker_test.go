package memorycache

import (
	"testing"
	"time"

	"github.com/patrickmn/go-cache"
)

func TestMemoryCacheBroker_Exec(t *testing.T) {
	t.Run("fetches data from origin when cache is empty", func(t *testing.T) {
		cacheKey := "unique-key-for-cache-miss"
		expiration := 25 * time.Millisecond
		sharedCache := cache.New(DefaultExpiration, DefaultCleanupInterval)
		broker := NewMemoryCacheBroker[any](cacheKey, expiration, WithCacheClient(sharedCache))
		broker.Clear()

		called := false

		_, err := broker.Exec(func() (any, error) {
			called = true
			return struct{}{}, nil
		})
		if err != nil {
			t.Fatalf("unexpected error during Exec: %v", err)
		}
		if !called {
			t.Errorf("expected getData to be called when cache is empty, but it was not")
		}
	})

	t.Run("returns cached data when present", func(t *testing.T) {
		cacheKey := "key-for-cached-data"
		expiration := 5 * time.Second
		sharedCache := cache.New(DefaultExpiration, DefaultCleanupInterval)
		broker := NewMemoryCacheBroker[any](cacheKey, expiration, WithCacheClient(sharedCache))
		broker.Clear()

		_, err := broker.Exec(func() (any, error) {
			return "cached", nil
		})
		if err != nil {
			t.Fatalf("unexpected error during first Exec call: %v", err)
		}

		called := false
		_, err = broker.Exec(func() (any, error) {
			called = true
			return "should not be called", nil
		})
		if err != nil {
			t.Fatalf("unexpected error during second Exec call: %v", err)
		}
		if called {
			t.Errorf("expected cached data to be returned, but getData was called")
		}
	})
}
