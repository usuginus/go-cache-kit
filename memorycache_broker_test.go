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

	t.Run("accepts context TODO", func(t *testing.T) {
		cacheKey := "key-for-exec-context-todo-context"
		sharedCache := cache.New(DefaultExpiration, DefaultCleanupInterval)
		broker, err := NewMemoryCacheBroker[string](cacheKey, 1*time.Second, WithCacheClient(sharedCache))
		if err != nil {
			t.Fatalf("failed to create broker: %v", err)
		}
		broker.Clear()

		value, err := broker.ExecContext(context.TODO(), func(inner context.Context) (string, error) {
			if inner == nil {
				t.Fatalf("expected non-nil context")
			}
			return "from-todo-ctx", nil
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if value != "from-todo-ctx" {
			t.Fatalf("unexpected value: %q", value)
		}
	})

	t.Run("returns cached value even when caller context is already canceled", func(t *testing.T) {
		cacheKey := "key-for-exec-context-canceled-but-hit"
		sharedCache := cache.New(DefaultExpiration, DefaultCleanupInterval)
		broker, err := NewMemoryCacheBroker[string](cacheKey, 1*time.Second, WithCacheClient(sharedCache))
		if err != nil {
			t.Fatalf("failed to create broker: %v", err)
		}
		broker.Clear()

		if _, err := broker.ExecContext(context.Background(), func(context.Context) (string, error) {
			return "cached-value", nil
		}); err != nil {
			t.Fatalf("failed to warm cache: %v", err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		called := false
		value, err := broker.ExecContext(ctx, func(context.Context) (string, error) {
			called = true
			return "should-not-run", nil
		})
		if err != nil {
			t.Fatalf("expected cache hit even with canceled context, got %v", err)
		}
		if called {
			t.Fatalf("expected fetcher not to run on cache hit")
		}
		if value != "cached-value" {
			t.Fatalf("unexpected cached value: %q", value)
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

	t.Run("deduplicates concurrent cache misses", func(t *testing.T) {
		cacheKey := "key-for-exec-context-concurrent-miss-dedup"
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

		type ctxKey string
		const k ctxKey = "request-id"
		ctx := context.WithValue(context.Background(), k, "req-456")

		for range workers {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				value, err := broker.ExecContext(ctx, func(inner context.Context) (string, error) {
					atomic.AddInt32(&calls, 1)
					time.Sleep(30 * time.Millisecond)
					v, _ := inner.Value(k).(string)
					if v != "req-456" {
						return "", errors.New("context value not propagated correctly")
					}
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
			t.Fatalf("unexpected concurrent ExecContext error: %v", err)
		}

		if got := atomic.LoadInt32(&calls); got != 1 {
			t.Fatalf("expected origin function to be called once, got %d", got)
		}
	})

	t.Run("propagates shared origin error to all waiters and does not cache", func(t *testing.T) {
		cacheKey := "key-for-exec-context-shared-origin-error"
		sharedCache := cache.New(DefaultExpiration, DefaultCleanupInterval)
		broker, err := NewMemoryCacheBroker[string](cacheKey, 1*time.Second, WithCacheClient(sharedCache))
		if err != nil {
			t.Fatalf("failed to create broker: %v", err)
		}
		broker.Clear()

		originErr := errors.New("origin failed")
		const workers = 8
		start := make(chan struct{})
		errCh := make(chan error, workers)
		var calls int32
		var wg sync.WaitGroup

		for range workers {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				_, err := broker.ExecContext(context.Background(), func(context.Context) (string, error) {
					atomic.AddInt32(&calls, 1)
					time.Sleep(20 * time.Millisecond)
					return "", originErr
				})
				errCh <- err
			}()
		}

		close(start)
		wg.Wait()
		close(errCh)

		for gotErr := range errCh {
			if !errors.Is(gotErr, originErr) {
				t.Fatalf("expected originErr, got %v", gotErr)
			}
		}
		if got := atomic.LoadInt32(&calls); got != 1 {
			t.Fatalf("expected origin function to be called once, got %d", got)
		}
		if _, err := broker.provider.Get(); !errors.Is(err, ErrDataNotFound) {
			t.Fatalf("expected no cached value after shared origin error, got %v", err)
		}
	})

	t.Run("allows waiter context cancellation while miss is in-flight", func(t *testing.T) {
		cacheKey := "key-for-exec-context-waiter-cancel"
		sharedCache := cache.New(DefaultExpiration, DefaultCleanupInterval)
		broker, err := NewMemoryCacheBroker[string](cacheKey, 1*time.Second, WithCacheClient(sharedCache))
		if err != nil {
			t.Fatalf("failed to create broker: %v", err)
		}
		broker.Clear()

		leaderStarted := make(chan struct{})
		releaseLeader := make(chan struct{})
		leaderDone := make(chan struct {
			value string
			err   error
		}, 1)

		var calls int32
		go func() {
			value, err := broker.ExecContext(context.Background(), func(context.Context) (string, error) {
				atomic.AddInt32(&calls, 1)
				close(leaderStarted)
				<-releaseLeader
				return "leader", nil
			})
			leaderDone <- struct {
				value string
				err   error
			}{value: value, err: err}
		}()

		<-leaderStarted

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()

		waitStart := time.Now()
		_, err = broker.ExecContext(ctx, func(context.Context) (string, error) {
			atomic.AddInt32(&calls, 1)
			return "waiter", nil
		})
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("expected context.DeadlineExceeded, got %v", err)
		}
		if elapsed := time.Since(waitStart); elapsed > 80*time.Millisecond {
			t.Fatalf("waiter cancellation was too slow: %v", elapsed)
		}

		close(releaseLeader)
		leaderResult := <-leaderDone
		if leaderResult.err != nil {
			t.Fatalf("leader failed: %v", leaderResult.err)
		}
		if leaderResult.value != "leader" {
			t.Fatalf("unexpected leader value: %q", leaderResult.value)
		}

		if got := atomic.LoadInt32(&calls); got != 1 {
			t.Fatalf("expected only leader fetch to run, got %d calls", got)
		}
	})

	t.Run("keeps in-flight fetch running when one caller cancels", func(t *testing.T) {
		cacheKey := "key-for-exec-context-leader-cancel"
		sharedCache := cache.New(DefaultExpiration, DefaultCleanupInterval)
		broker, err := NewMemoryCacheBroker[string](cacheKey, 1*time.Second, WithCacheClient(sharedCache))
		if err != nil {
			t.Fatalf("failed to create broker: %v", err)
		}
		broker.Clear()

		leaderStarted := make(chan struct{})
		releaseFetch := make(chan struct{})
		leaderErrCh := make(chan error, 1)
		followerCh := make(chan struct {
			value string
			err   error
		}, 1)

		var calls int32
		leaderCtx, cancelLeader := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancelLeader()

		go func() {
			_, err := broker.ExecContext(leaderCtx, func(context.Context) (string, error) {
				atomic.AddInt32(&calls, 1)
				close(leaderStarted)
				<-releaseFetch
				return "shared-value", nil
			})
			leaderErrCh <- err
		}()

		<-leaderStarted

		go func() {
			value, err := broker.ExecContext(context.Background(), func(context.Context) (string, error) {
				atomic.AddInt32(&calls, 1)
				return "follower-should-not-fetch", nil
			})
			followerCh <- struct {
				value string
				err   error
			}{value: value, err: err}
		}()

		select {
		case err := <-leaderErrCh:
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("expected leader to return context.DeadlineExceeded, got %v", err)
			}
		case <-time.After(200 * time.Millisecond):
			t.Fatalf("leader did not return after cancellation")
		}

		close(releaseFetch)

		select {
		case result := <-followerCh:
			if result.err != nil {
				t.Fatalf("follower failed: %v", result.err)
			}
			if result.value != "shared-value" {
				t.Fatalf("unexpected follower value: %q", result.value)
			}
		case <-time.After(300 * time.Millisecond):
			t.Fatalf("follower did not receive shared result in time")
		}

		if got := atomic.LoadInt32(&calls); got != 1 {
			t.Fatalf("expected origin function to be called once, got %d", got)
		}
	})

	t.Run("cancels shared fetch when all callers cancel", func(t *testing.T) {
		cacheKey := "key-for-exec-context-all-cancel"
		sharedCache := cache.New(DefaultExpiration, DefaultCleanupInterval)
		broker, err := NewMemoryCacheBroker[string](cacheKey, 1*time.Second, WithCacheClient(sharedCache))
		if err != nil {
			t.Fatalf("failed to create broker: %v", err)
		}
		broker.Clear()

		fetchDone := make(chan error, 1)
		var calls int32

		leaderCtx, cancelLeader := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancelLeader()
		followerCtx, cancelFollower := context.WithTimeout(context.Background(), 30*time.Millisecond)
		defer cancelFollower()

		leaderErrCh := make(chan error, 1)
		followerErrCh := make(chan error, 1)

		getData := func(inner context.Context) (string, error) {
			atomic.AddInt32(&calls, 1)
			<-inner.Done()
			err := inner.Err()
			fetchDone <- err
			return "", err
		}

		go func() {
			_, err := broker.ExecContext(leaderCtx, getData)
			leaderErrCh <- err
		}()
		go func() {
			_, err := broker.ExecContext(followerCtx, getData)
			followerErrCh <- err
		}()

		select {
		case err := <-leaderErrCh:
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("expected leader deadline error, got %v", err)
			}
		case <-time.After(200 * time.Millisecond):
			t.Fatalf("leader did not return in time")
		}
		select {
		case err := <-followerErrCh:
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("expected follower deadline error, got %v", err)
			}
		case <-time.After(200 * time.Millisecond):
			t.Fatalf("follower did not return in time")
		}
		select {
		case err := <-fetchDone:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("expected shared fetch context.Canceled, got %v", err)
			}
		case <-time.After(300 * time.Millisecond):
			t.Fatalf("shared fetch was not canceled after all callers canceled")
		}

		if got := atomic.LoadInt32(&calls); got != 1 {
			t.Fatalf("expected origin function to be called once, got %d", got)
		}
		if _, err := broker.provider.Get(); !errors.Is(err, ErrDataNotFound) {
			t.Fatalf("expected no cached value after all callers canceled, got %v", err)
		}

		freshValue, err := broker.ExecContext(context.Background(), func(context.Context) (string, error) {
			atomic.AddInt32(&calls, 1)
			return "fresh-after-cancel", nil
		})
		if err != nil {
			t.Fatalf("expected fresh fetch to succeed after all canceled, got %v", err)
		}
		if freshValue != "fresh-after-cancel" {
			t.Fatalf("unexpected fresh value: %q", freshValue)
		}
		if got := atomic.LoadInt32(&calls); got != 2 {
			t.Fatalf("expected origin function total calls to be 2, got %d", got)
		}
	})

	t.Run("clears in-flight state when fetcher panics and returns wrapped error", func(t *testing.T) {
		cacheKey := "key-for-exec-context-panic-recovery"
		sharedCache := cache.New(DefaultExpiration, DefaultCleanupInterval)
		broker, err := NewMemoryCacheBroker[string](cacheKey, 1*time.Second, WithCacheClient(sharedCache))
		if err != nil {
			t.Fatalf("failed to create broker: %v", err)
		}
		broker.Clear()

		_, err = broker.ExecContext(context.Background(), func(context.Context) (string, error) {
			panic("boom")
		})
		if !errors.Is(err, ErrDataFetcherPanicked) {
			t.Fatalf("expected ErrDataFetcherPanicked, got %v", err)
		}

		value, err := broker.ExecContext(context.Background(), func(context.Context) (string, error) {
			return "recovered", nil
		})
		if err != nil {
			t.Fatalf("unexpected error after panic recovery: %v", err)
		}
		if value != "recovered" {
			t.Fatalf("unexpected value after panic recovery: %q", value)
		}
	})
}
