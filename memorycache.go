package memorycache

import (
	"fmt"
	"time"

	"github.com/patrickmn/go-cache"
)

// DefaultExpiration is the default duration for cache item expiration.
const DefaultExpiration = 10 * time.Second

// DefaultCleanupInterval is the interval at which expired cache items are cleaned up.
const DefaultCleanupInterval = 60 * time.Second

var (
	// ErrDataNotFound is returned when the requested cache entry does not exist.
	ErrDataNotFound = fmt.Errorf("cache data not found")
	// ErrAssertionFailed is returned when a cached value cannot be type-asserted to the expected type.
	ErrAssertionFailed = fmt.Errorf("cache data type assertion failed")
)

// defaultCacheClient is the underlying shared in-memory cache instance.
var defaultCacheClient = cache.New(DefaultExpiration, DefaultCleanupInterval)

// ProviderOption configures a MemoryCacheProvider.
type ProviderOption func(*providerConfig)

type providerConfig struct {
	client *cache.Cache
}

// WithCacheClient lets callers inject a custom cache client.
func WithCacheClient(client *cache.Cache) ProviderOption {
	return func(cfg *providerConfig) {
		cfg.client = client
	}
}

// WithCacheConfig creates a dedicated cache client using the supplied settings.
// This option overrides WithCacheClient when both are provided.
func WithCacheConfig(defaultExpiration, cleanupInterval time.Duration) ProviderOption {
	return func(cfg *providerConfig) {
		cfg.client = cache.New(defaultExpiration, cleanupInterval)
	}
}

// MemoryCacheProvider provides a generic interface for caching values of type T.
type MemoryCacheProvider[T any] struct {
	cacheKey string
	client   *cache.Cache
}

// NewMemoryCacheProvider creates a new MemoryCacheProvider with the specified cache key.
func NewMemoryCacheProvider[T any](cacheKey string, opts ...ProviderOption) *MemoryCacheProvider[T] {
	cfg := providerConfig{client: defaultCacheClient}
	for _, opt := range opts {
		opt(&cfg)
	}
	if cfg.client == nil {
		cfg.client = defaultCacheClient
	}
	return &MemoryCacheProvider[T]{
		cacheKey: cacheKey,
		client:   cfg.client,
	}
}

// Get retrieves the cached value associated with the provider's cache key.
// If the cache entry is missing or if the type assertion fails, an error is returned.
func (m *MemoryCacheProvider[T]) Get() (T, error) {
	var result T
	raw, found := m.client.Get(m.cacheKey)
	if !found {
		return result, ErrDataNotFound
	}
	value, ok := raw.(T)
	if !ok {
		return result, ErrAssertionFailed
	}
	return value, nil
}

// Set caches the given value with a custom time-to-live (TTL).
func (m *MemoryCacheProvider[T]) Set(value T, ttl time.Duration) {
	m.client.Set(m.cacheKey, value, ttl)
}

// SetDefault caches the given value using the default expiration time.
func (m *MemoryCacheProvider[T]) SetDefault(value T) {
	m.client.SetDefault(m.cacheKey, value)
}

// Clear removes the cached entry associated with the provider's cache key.
func (m *MemoryCacheProvider[T]) Clear() {
	m.client.Delete(m.cacheKey)
}
