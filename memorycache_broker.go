package memorycache

import (
	"errors"
	"time"
)

// MemoryCacheBroker provides a transparent caching layer by leveraging the underlying cache provider.
// It wraps a cache key and TTL configuration for caching operations.
type MemoryCacheBroker[T any] struct {
	ttl      time.Duration
	provider *MemoryCacheProvider[T]
}

// NewMemoryCacheBroker creates a new MemoryCacheBroker with the specified key and TTL.
func NewMemoryCacheBroker[T any](key string, ttl time.Duration, opts ...ProviderOption) *MemoryCacheBroker[T] {
	provider := NewMemoryCacheProvider[T](key, opts...)
	return &MemoryCacheBroker[T]{
		ttl:      ttl,
		provider: provider,
	}
}

// Exec executes the provided data-fetching function through the cache.
// It first attempts to retrieve the data from the cache using the broker's key.
// If the data is not present, it calls getData to fetch the data from the source,
// then stores the result in the cache with the broker's TTL.
func (b *MemoryCacheBroker[T]) Exec(getData func() (T, error)) (T, error) {
	if cachedData, err := b.provider.Get(); err == nil {
		return cachedData, nil
	} else if !errors.Is(err, ErrDataNotFound) {
		var zero T
		return zero, err
	}

	fetchedData, err := getData()
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
