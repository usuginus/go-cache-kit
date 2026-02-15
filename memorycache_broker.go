package memorycache

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/patrickmn/go-cache"
)

// MemoryCacheBroker provides a transparent caching layer by leveraging the underlying cache provider.
// It wraps a cache key and TTL configuration for caching operations.
type MemoryCacheBroker[T any] struct {
	ttl      time.Duration
	provider *MemoryCacheProvider[T]
	mu       sync.Mutex
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

	if cachedData, err := b.provider.Get(); err == nil {
		return cachedData, nil
	} else if !errors.Is(err, ErrDataNotFound) {
		var zero T
		return zero, err
	}

	// Ensure only one concurrent cache miss fetches origin data for this broker key.
	b.mu.Lock()
	defer b.mu.Unlock()

	if cachedData, err := b.provider.Get(); err == nil {
		return cachedData, nil
	} else if !errors.Is(err, ErrDataNotFound) {
		var zero T
		return zero, err
	}

	fetchedData, err := getData(ctx)
	if err != nil {
		var zero T
		return zero, err
	}

	b.provider.Set(fetchedData, b.ttl)

	return fetchedData, nil
}

// Clear removes the cached data associated with the broker's key.
// This is mainly used for testing purposes.
func (b *MemoryCacheBroker[T]) Clear() {
	b.provider.Clear()
}
