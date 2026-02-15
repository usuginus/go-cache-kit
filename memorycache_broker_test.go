package memorycache

import (
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

	t.Run("returns error when data fetcher is nil", func(t *testing.T) {
		cacheKey := "key-for-nil-fetcher"
		sharedCache := cache.New(DefaultExpiration, DefaultCleanupInterval)
		broker := NewMemoryCacheBroker[any](cacheKey, 1*time.Second, WithCacheClient(sharedCache))
		broker.Clear()

		if _, err := broker.Exec(nil); !errors.Is(err, ErrNilDataFetcher) {
			t.Fatalf("expected ErrNilDataFetcher, got %v", err)
		}
	})

	t.Run("does not cache failed origin result", func(t *testing.T) {
		cacheKey := "key-for-origin-error"
		sharedCache := cache.New(DefaultExpiration, DefaultCleanupInterval)
		broker := NewMemoryCacheBroker[any](cacheKey, 1*time.Second, WithCacheClient(sharedCache))
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
		broker := NewMemoryCacheBroker[string](cacheKey, 1*time.Second, WithCacheClient(sharedCache))
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
}
