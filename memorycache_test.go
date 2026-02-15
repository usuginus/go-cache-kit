package memorycache

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/mattn/go-nulltype"
	"github.com/patrickmn/go-cache"
)

// ExampleStruct is used for testing the caching of structured data.
type ExampleStruct struct {
	ExampleInt    int
	ExampleString string
	ExampleTime   time.Time
	ExampleStruct nulltype.NullInt64
}

func TestMemoryCacheProvider_Get(t *testing.T) {
	newTestProvider := func(key string) *MemoryCacheProvider[ExampleStruct] {
		customCache := cache.New(50*time.Millisecond, 50*time.Millisecond)
		return NewMemoryCacheProvider[ExampleStruct](key, WithCacheClient(customCache))
	}

	t.Run("Cache Hit", func(t *testing.T) {
		key := "cache-key:example1"
		value := ExampleStruct{
			ExampleInt:    1,
			ExampleString: "test",
			ExampleTime:   time.Date(2022, 4, 1, 0, 0, 0, 0, time.Local),
			ExampleStruct: nulltype.NullInt64Of(1),
		}
		ttl := 25 * time.Millisecond

		provider := newTestProvider(key)
		provider.Set(value, ttl)

		cachedValue, err := provider.Get()
		if err != nil {
			t.Fatalf("Cache Hit: unexpected error: %v", err)
		}
		if !reflect.DeepEqual(cachedValue, value) {
			t.Errorf("Cache Hit: got %+v, want %+v", cachedValue, value)
		}
	})

	t.Run("Cache Miss", func(t *testing.T) {
		key := "cache-key:example2"
		provider := newTestProvider(key)

		if _, err := provider.Get(); err == nil {
			t.Error("Cache Miss: expected error for missing cache data, got nil")
		} else if err != ErrDataNotFound {
			t.Errorf("Cache Miss: expected error %v, got %v", ErrDataNotFound, err)
		}
	})

	t.Run("Cache Expiration", func(t *testing.T) {
		key := "cache-key:example3"
		value := ExampleStruct{
			ExampleInt:    1,
			ExampleString: "test",
			ExampleTime:   time.Date(2022, 4, 1, 0, 0, 0, 0, time.Local),
			ExampleStruct: nulltype.NullInt64Of(1),
		}
		ttl := 40 * time.Millisecond

		provider := newTestProvider(key)
		provider.Set(value, ttl)

		if _, err := provider.Get(); err != nil {
			t.Fatalf("Cache Expiration: unexpected error retrieving cache: %v", err)
		}

		time.Sleep(2 * ttl)

		if _, err := provider.Get(); err == nil {
			t.Error("Cache Expiration: expected error for expired cache data, got nil")
		} else if err != ErrDataNotFound {
			t.Errorf("Cache Expiration: expected error %v, got %v", ErrDataNotFound, err)
		}
	})
}

func TestMemoryCacheProvider_Get_TypeAssertionFailed(t *testing.T) {
	key := "cache-key:type-assertion-failed"
	customCache := cache.New(1*time.Minute, 1*time.Minute)
	customCache.Set(key, "invalid-type", 1*time.Minute)

	provider := NewMemoryCacheProvider[ExampleStruct](key, WithCacheClient(customCache))
	if _, err := provider.Get(); !errors.Is(err, ErrAssertionFailed) {
		t.Fatalf("expected ErrAssertionFailed, got %v", err)
	}
}

func TestMemoryCacheProvider_WithCacheConfigOverridesWithCacheClient(t *testing.T) {
	sharedCache := cache.New(1*time.Minute, 1*time.Minute)

	key1 := "cache-key:override-order1"
	provider1 := NewMemoryCacheProvider[string](
		key1,
		WithCacheClient(sharedCache),
		WithCacheConfig(1*time.Minute, 1*time.Minute),
	)
	provider1.Set("value-1", 1*time.Minute)
	if _, found := sharedCache.Get(key1); found {
		t.Fatalf("expected dedicated cache client for order1, but value leaked into shared cache")
	}

	key2 := "cache-key:override-order2"
	provider2 := NewMemoryCacheProvider[string](
		key2,
		WithCacheConfig(1*time.Minute, 1*time.Minute),
		WithCacheClient(sharedCache),
	)
	provider2.Set("value-2", 1*time.Minute)
	if _, found := sharedCache.Get(key2); found {
		t.Fatalf("expected dedicated cache client for order2, but value leaked into shared cache")
	}
}
