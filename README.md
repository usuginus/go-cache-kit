# go-cache-kit

`go-cache-kit` is a lightweight, generic in-memory cache helper for Go.
It wraps [go-cache](https://github.com/patrickmn/go-cache) with type-safe APIs and a cache-aside broker.

## Features

- Type-safe cache operations with Go generics
- Cache-aside execution via `MemoryCacheBroker.Exec`
- Context-aware cache-aside execution via `MemoryCacheBroker.ExecContext`
- Waiters can cancel while a cache-miss fetch is in-flight
- Shared miss fetch is canceled when all waiting callers cancel
- Concurrent miss de-duplication per broker instance
- Flexible cache configuration via options
- Constructor input validation (`key`, `ttl`, and nil custom client checks)

## Installation

```bash
go get github.com/usuginus/go-cache-kit
```

```go
import memorycache "github.com/usuginus/go-cache-kit"
```

## Quick Start

### MemoryCacheProvider

```go
package main

import (
	"fmt"
	"time"

	memorycache "github.com/usuginus/go-cache-kit"
)

func main() {
	provider, err := memorycache.NewMemoryCacheProvider[int]("example:key")
	if err != nil {
		fmt.Println("provider init error:", err)
		return
	}

	provider.Set(42, 10*time.Second)

	value, err := provider.Get()
	if err != nil {
		fmt.Println("provider get error:", err)
		return
	}

	fmt.Println("cached value:", value)
}
```

### MemoryCacheBroker

```go
package main

import (
	"fmt"
	"time"

	memorycache "github.com/usuginus/go-cache-kit"
)

func main() {
	broker, err := memorycache.NewMemoryCacheBroker[string]("example:broker", 30*time.Second)
	if err != nil {
		fmt.Println("broker init error:", err)
		return
	}

	value, err := broker.Exec(func() (string, error) {
		return "Hello, Cached World!", nil
	})
	if err != nil {
		fmt.Println("broker exec error:", err)
		return
	}

	fmt.Println("result:", value)
}
```

### MemoryCacheBroker with context timeout

```go
package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	memorycache "github.com/usuginus/go-cache-kit"
)

func main() {
	broker, err := memorycache.NewMemoryCacheBroker[string]("example:ctx", 30*time.Second)
	if err != nil {
		fmt.Println("broker init error:", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	value, err := broker.ExecContext(ctx, func(ctx context.Context) (string, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.com/api/value", nil)
		if err != nil {
			return "", err
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("unexpected status: %s", resp.Status)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", err
		}

		return string(body), nil
	})
	if err != nil {
		fmt.Println("broker exec context error:", err)
		return
	}

	fmt.Println("result:", value)
}
```

## ExecContext Semantics

- On cache hit, cached data is returned immediately.
- On cache miss, concurrent callers on the same broker share one origin fetch.
- Each caller can stop waiting via its own context.
- The shared origin fetch is canceled only when all waiting callers are canceled.

## Options

- `WithCacheClient(client)`: reuse an existing `*cache.Cache` instance
- `WithCacheConfig(defaultExpiration, cleanupInterval)`: create a dedicated cache client with custom settings
- `WithIsolatedCache()`: create a dedicated cache client with package defaults

`WithCacheConfig(...)` takes precedence over `WithCacheClient(...)` when both are passed.

By default, providers share one package-level cache instance.
Use unique keys across your application when relying on this shared default.

## Validation Rules

- `NewMemoryCacheProvider(...)` rejects empty/whitespace keys (`ErrInvalidCacheKey`)
- `WithCacheClient(nil)` is rejected (`ErrNilCacheClient`)
- `NewMemoryCacheBroker(...)` rejects invalid TTL (`ErrInvalidCacheTTL`)
- `MemoryCacheBroker.Exec(nil)` is rejected (`ErrNilDataFetcher`)
- `MemoryCacheBroker.ExecContext(..., nil)` is rejected (`ErrNilDataFetcher`)
- Panics in broker fetcher are returned as `ErrDataFetcherPanicked`

`ExecContext` notes:
- Cache hit is prioritized even if caller context is already canceled.
- On cache miss with an already-canceled context, `ctx.Err()` is returned.

TTL is valid when it is positive, `cache.DefaultExpiration`, or `cache.NoExpiration`.

## Benchmark

Run benchmarks with allocation stats:

```bash
go test -run '^$' -bench . -benchmem ./...
```

Sample results (Go 1.22.2, darwin/arm64):

- `Provider.Set`: `70.29 ns/op`
- `Provider.Get` (hit): `54.04 ns/op`
- `Provider.Get` (miss): `14.34 ns/op`
- `Broker.Exec` (hit): `47.75 ns/op`
- `Broker.Exec` (miss): `138.4 ns/op`
- `Broker.Exec` (hit, parallel): `92.74 ns/op`

## License

MIT License. See [LICENSE](LICENSE).
