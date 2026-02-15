package memorycache

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/patrickmn/go-cache"
)

func TestMemoryCacheBroker_Exec(t *testing.T) {
	t.Run("fetches data from origin when cache is empty", func(t *testing.T) {
		cacheKey := "unique-key-for-cache-miss"
		expiration := 25 * time.Millisecond
		sharedCache := cache.New(DefaultExpiration, DefaultCleanupInterval)
		broker, err := NewMemoryCacheBroker[any](cacheKey, expiration, WithCacheClient(sharedCache))
		if err != nil {
			t.Fatalf("failed to create broker: %v", err)
		}
		broker.Clear()

		called := false

		_, err = broker.Exec(func() (any, error) {
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
		broker, err := NewMemoryCacheBroker[any](cacheKey, expiration, WithCacheClient(sharedCache))
		if err != nil {
			t.Fatalf("failed to create broker: %v", err)
		}
		broker.Clear()

		_, err = broker.Exec(func() (any, error) {
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

	t.Run("returns error when data fetcher is nil", func(t *testing.T) {
		cacheKey := "key-for-nil-fetcher"
		sharedCache := cache.New(DefaultExpiration, DefaultCleanupInterval)
		broker, err := NewMemoryCacheBroker[any](cacheKey, 1*time.Second, WithCacheClient(sharedCache))
		if err != nil {
			t.Fatalf("failed to create broker: %v", err)
		}
		broker.Clear()

		if _, err := broker.Exec(nil); !errors.Is(err, ErrNilDataFetcher) {
			t.Fatalf("expected ErrNilDataFetcher, got %v", err)
		}
	})

	t.Run("does not cache failed origin result", func(t *testing.T) {
		cacheKey := "key-for-origin-error"
		sharedCache := cache.New(DefaultExpiration, DefaultCleanupInterval)
		broker, err := NewMemoryCacheBroker[any](cacheKey, 1*time.Second, WithCacheClient(sharedCache))
		if err != nil {
			t.Fatalf("failed to create broker: %v", err)
		}
		broker.Clear()

		originErr := errors.New("origin failure")
		calls := 0

		if _, err := broker.Exec(func() (any, error) {
			calls++
			return nil, originErr
		}); !errors.Is(err, originErr) {
			t.Fatalf("expected origin error, got %v", err)
		}

		value, err := broker.Exec(func() (any, error) {
			calls++
			return "fresh", nil
		})
		if err != nil {
			t.Fatalf("unexpected error on second Exec: %v", err)
		}
		if value != "fresh" {
			t.Fatalf("expected fresh value, got %v", value)
		}
		if calls != 2 {
			t.Fatalf("expected origin function to be called twice, got %d", calls)
		}
	})

	t.Run("deduplicates concurrent cache misses", func(t *testing.T) {
		cacheKey := "key-for-concurrent-miss-dedup"
		sharedCache := cache.New(DefaultExpiration, DefaultCleanupInterval)
		broker, err := NewMemoryCacheBroker[string](cacheKey, 1*time.Second, WithCacheClient(sharedCache))
		if err != nil {
			t.Fatalf("failed to create broker: %v", err)
		}
		broker.Clear()

		const workers = 16
		start := make(chan struct{})
		errCh := make(chan error, workers)
		var calls int32
		var wg sync.WaitGroup

		for range workers {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				value, err := broker.Exec(func() (string, error) {
					atomic.AddInt32(&calls, 1)
					time.Sleep(30 * time.Millisecond)
					return "deduped", nil
				})
				if err != nil {
					errCh <- err
					return
				}
				if value != "deduped" {
					errCh <- errors.New("unexpected value returned from cache broker")
				}
			}()
		}

		close(start)
		wg.Wait()
		close(errCh)

		for err := range errCh {
			t.Fatalf("unexpected concurrent Exec error: %v", err)
		}

		if got := atomic.LoadInt32(&calls); got != 1 {
			t.Fatalf("expected origin function to be called once, got %d", got)
		}
	})

	t.Run("rejects empty key", func(t *testing.T) {
		if _, err := NewMemoryCacheBroker[string]("   ", 1*time.Second); !errors.Is(err, ErrInvalidCacheKey) {
			t.Fatalf("expected ErrInvalidCacheKey, got %v", err)
		}
	})

	t.Run("rejects invalid ttl", func(t *testing.T) {
		if _, err := NewMemoryCacheBroker[string]("valid-key", -2*time.Second); !errors.Is(err, ErrInvalidCacheTTL) {
			t.Fatalf("expected ErrInvalidCacheTTL, got %v", err)
		}
	})
}

func TestMemoryCacheBroker_ExecContext(t *testing.T) {
	t.Run("passes context to data fetcher on cache miss", func(t *testing.T) {
		cacheKey := "key-for-exec-context-miss"
		sharedCache := cache.New(DefaultExpiration, DefaultCleanupInterval)
		broker, err := NewMemoryCacheBroker[string](cacheKey, 1*time.Second, WithCacheClient(sharedCache))
		if err != nil {
			t.Fatalf("failed to create broker: %v", err)
		}
		broker.Clear()

		type ctxKey string
		const k ctxKey = "trace-id"
		ctx := context.WithValue(context.Background(), k, "abc-123")

		value, err := broker.ExecContext(ctx, func(inner context.Context) (string, error) {
			v, _ := inner.Value(k).(string)
			if v != "abc-123" {
				t.Fatalf("unexpected context value: %q", v)
			}
			return "from-origin", nil
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if value != "from-origin" {
			t.Fatalf("unexpected value: %q", value)
		}
	})

	t.Run("returns cached data and skips data fetcher when present", func(t *testing.T) {
		cacheKey := "key-for-exec-context-hit"
		sharedCache := cache.New(DefaultExpiration, DefaultCleanupInterval)
		broker, err := NewMemoryCacheBroker[string](cacheKey, 1*time.Second, WithCacheClient(sharedCache))
		if err != nil {
			t.Fatalf("failed to create broker: %v", err)
		}
		broker.Clear()

		if _, err := broker.ExecContext(context.Background(), func(context.Context) (string, error) {
			return "cached", nil
		}); err != nil {
			t.Fatalf("unexpected error on warm-up: %v", err)
		}

		called := false
		value, err := broker.ExecContext(context.Background(), func(context.Context) (string, error) {
			called = true
			return "should-not-run", nil
		})
		if err != nil {
			t.Fatalf("unexpected error on cached call: %v", err)
		}
		if called {
			t.Fatalf("expected data fetcher not to be called on cache hit")
		}
		if value != "cached" {
			t.Fatalf("unexpected cached value: %q", value)
		}
	})

	t.Run("returns error when data fetcher is nil", func(t *testing.T) {
		cacheKey := "key-for-exec-context-nil-fetcher"
		sharedCache := cache.New(DefaultExpiration, DefaultCleanupInterval)
		broker, err := NewMemoryCacheBroker[string](cacheKey, 1*time.Second, WithCacheClient(sharedCache))
		if err != nil {
			t.Fatalf("failed to create broker: %v", err)
		}
		broker.Clear()

		if _, err := broker.ExecContext(context.Background(), nil); !errors.Is(err, ErrNilDataFetcher) {
			t.Fatalf("expected ErrNilDataFetcher, got %v", err)
		}
	})

	t.Run("propagates context cancellation error and does not cache", func(t *testing.T) {
		cacheKey := "key-for-exec-context-canceled"
		sharedCache := cache.New(DefaultExpiration, DefaultCleanupInterval)
		broker, err := NewMemoryCacheBroker[string](cacheKey, 1*time.Second, WithCacheClient(sharedCache))
		if err != nil {
			t.Fatalf("failed to create broker: %v", err)
		}
		broker.Clear()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		if _, err := broker.ExecContext(ctx, func(context.Context) (string, error) {
			return "", context.Canceled
		}); !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}

		if _, err := broker.provider.Get(); !errors.Is(err, ErrDataNotFound) {
			t.Fatalf("expected no cached value after canceled fetch, got %v", err)
		}
	})

	t.Run("handles nil context by converting to context.Background", func(t *testing.T) {
		cacheKey := "key-for-exec-context-nil"
		sharedCache := cache.New(DefaultExpiration, DefaultCleanupInterval)
		broker, err := NewMemoryCacheBroker[string](cacheKey, 1*time.Second, WithCacheClient(sharedCache))
		if err != nil {
			t.Fatalf("failed to create broker: %v", err)
		}
		broker.Clear()

		called := false
		value, err := broker.ExecContext(nil, func(ctx context.Context) (string, error) {
			called = true
			if ctx == nil {
				t.Fatal("expected non-nil context in data fetcher")
			}
			return "success", nil
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !called {
			t.Fatal("expected data fetcher to be called")
		}
		if value != "success" {
			t.Fatalf("unexpected value: %q", value)
		}
	})
}
