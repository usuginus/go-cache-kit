package memorycache

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/patrickmn/go-cache"
)

// MemoryCacheBroker provides cache-aside behavior for a single cache key.
//
// A broker first reads from cache and, on miss, executes the origin fetcher and
// stores the result with the broker TTL. Concurrent cache misses are coalesced:
// only one fetch runs at a time per broker instance.
//
// Cancellation semantics:
// - each caller can stop waiting via its own context
// - the shared fetch is canceled only when all current waiters cancel
type MemoryCacheBroker[T any] struct {
	ttl      time.Duration
	provider *MemoryCacheProvider[T]
	mu       sync.Mutex
	inFlight *inFlightCall[T]
}

// inFlightCall tracks one shared miss-fetch currently running for this broker.
type inFlightCall[T any] struct {
	fetchCtx context.Context
	cancel   context.CancelFunc
	done     chan struct{}
	waiters  int
	value    T
	err      error
}

// NewMemoryCacheBroker creates a new MemoryCacheBroker with the specified key and TTL.
func NewMemoryCacheBroker[T any](key string, ttl time.Duration, opts ...ProviderOption) (*MemoryCacheBroker[T], error) {
	if ttl <= 0 && ttl != cache.DefaultExpiration && ttl != cache.NoExpiration {
		return nil, ErrInvalidCacheTTL
	}

	provider, err := NewMemoryCacheProvider[T](key, opts...)
	if err != nil {
		return nil, err
	}

	return &MemoryCacheBroker[T]{
		ttl:      ttl,
		provider: provider,
	}, nil
}

// Exec executes the provided data-fetching function through the cache.
// It first attempts to retrieve the data from the cache using the broker's key.
// If the data is not present, it calls getData to fetch the data from the source,
// then stores the result in the cache with the broker's TTL.
func (b *MemoryCacheBroker[T]) Exec(getData func() (T, error)) (T, error) {
	if getData == nil {
		var zero T
		return zero, ErrNilDataFetcher
	}

	return b.ExecContext(context.Background(), func(context.Context) (T, error) {
		return getData()
	})
}

// ExecContext is the context-aware version of Exec.
// It first attempts to retrieve the data from the cache.
// On cache miss, it calls getData with ctx, stores the result with broker TTL, and returns it.
func (b *MemoryCacheBroker[T]) ExecContext(ctx context.Context, getData func(context.Context) (T, error)) (T, error) {
	if getData == nil {
		var zero T
		return zero, ErrNilDataFetcher
	}
	if ctx == nil {
		ctx = context.Background()
	}

	if cachedData, found, err := b.tryGetCached(); err != nil {
		var zero T
		return zero, err
	} else if found {
		return cachedData, nil
	}

	// Avoid creating/joining in-flight work for an already-canceled caller.
	if err := ctx.Err(); err != nil {
		var zero T
		return zero, err
	}

	var zero T
	b.mu.Lock()

	// Double-check cache under lock in case another goroutine filled it.
	if cachedData, found, err := b.tryGetCached(); err != nil {
		b.mu.Unlock()
		return zero, err
	} else if found {
		b.mu.Unlock()
		return cachedData, nil
	}

	// Join existing in-flight fetch for this broker/key.
	if call := b.inFlight; call != nil {
		call.waiters++
		b.mu.Unlock()
		return b.waitForInFlight(ctx, call)
	}

	// create a new shared fetch and register it as in-flight.
	call := newInFlightCall[T](ctx)
	b.inFlight = call
	b.mu.Unlock()

	// Run origin fetch outside lock so other callers can join/cancel promptly.
	go b.runInFlightFetch(call, getData)

	return b.waitForInFlight(ctx, call)
}

func (b *MemoryCacheBroker[T]) waitForInFlight(ctx context.Context, call *inFlightCall[T]) (T, error) {
	select {
	case <-call.done:
		return call.value, call.err
	case <-ctx.Done():
		// Prefer completed result when cancellation races with in-flight completion.
		select {
		case <-call.done:
			return call.value, call.err
		default:
		}

		b.releaseWaiter(call)
		var zero T
		return zero, ctx.Err()
	}
}

// runInFlightFetch executes the shared origin fetch and publishes the result to all waiters.
func (b *MemoryCacheBroker[T]) runInFlightFetch(call *inFlightCall[T], getData func(context.Context) (T, error)) {
	fetchedData, err := callFetcherWithRecover(call.fetchCtx, getData)
	if err == nil {
		b.provider.Set(fetchedData, b.ttl)
	}

	b.completeInFlight(call, fetchedData, err)
}

// completeInFlight stores the terminal result, unblocks waiters, and clears broker in-flight state.
func (b *MemoryCacheBroker[T]) completeInFlight(call *inFlightCall[T], value T, err error) {
	b.mu.Lock()
	call.value = value
	call.err = err
	close(call.done)
	if b.inFlight == call {
		b.inFlight = nil
	}
	b.mu.Unlock()

	call.cancel()
}

// releaseWaiter detaches one canceled caller from the current in-flight fetch.
func (b *MemoryCacheBroker[T]) releaseWaiter(call *inFlightCall[T]) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.inFlight != call || call.waiters == 0 {
		return
	}

	call.waiters--
	if call.waiters == 0 {
		// No callers are waiting for this shared fetch anymore.
		call.cancel()
	}
}

// tryGetCached returns (value, true, nil) on cache hit, (zero, false, nil) on miss,
// and (zero, false, err) on unexpected cache errors.
func (b *MemoryCacheBroker[T]) tryGetCached() (T, bool, error) {
	value, err := b.provider.Get()
	switch {
	case err == nil:
		return value, true, nil
	case errors.Is(err, ErrDataNotFound):
		var zero T
		return zero, false, nil
	default:
		var zero T
		return zero, false, err
	}
}

// newInFlightCall creates state for one shared miss-fetch.
func newInFlightCall[T any](ctx context.Context) *inFlightCall[T] {
	fetchCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	return &inFlightCall[T]{
		fetchCtx: fetchCtx,
		cancel:   cancel,
		done:     make(chan struct{}),
		waiters:  1,
	}
}

// callFetcherWithRecover converts a panic in getData into ErrDataFetcherPanicked.
func callFetcherWithRecover[T any](ctx context.Context, getData func(context.Context) (T, error)) (value T, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("%w: %v", ErrDataFetcherPanicked, recovered)
		}
	}()

	return getData(ctx)
}

// Clear removes the cached data associated with the broker's key.
// This is mainly used for testing purposes.
func (b *MemoryCacheBroker[T]) Clear() {
	b.provider.Clear()
}
