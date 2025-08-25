# go-cache-helper

go-cache-helper is a lightweight, generic caching helper for Go that simplifies the use of in-memory caching. It provides a type-safe cache provider and a broker that transparently executes functions through the cache, reducing repetitive cache management code in your applications.

## Overview

This repository contains the implementation of two primary components:

- MemoryCacheProvider:
A generic cache provider that wraps the underlying [go-cache](https://github.com/patrickmn/go-cache) library. It allows you to store, retrieve, and clear cache entries in a type-safe manner using Go generics.

- MemoryCacheBroker:
A caching broker that leverages the MemoryCacheProvider. It provides an Exec method which first attempts to retrieve data from the cache. If the data is not present, it executes a supplied data-fetching function, caches the result, and then returns it.

These components can help you implement transparent caching logic in your Go projects, making your business logic cleaner and more maintainable.

## Features

Type-Safe Caching: Use Go generics to store and retrieve any data type.
Configurable Expiration: Set custom expiration times for cache entries.
Transparent Execution: The broker automatically handles cache misses by invoking a data-fetching function.

## Installation

To install go-cache-helper, use go get:

```
go get github.com/usuginus/go-cache-helper
```

Then import the package in your code:

```
import "github.com/usuginus/go-cache-helper"
```

## Usage

Using MemoryCacheProvider

```
package main

import (
	"fmt"
	"time"

	memorycache "github.com/usuginus/go-cache-helper"
)

func main() {
	// Create a new MemoryCacheProvider for storing integer values.
	provider := memorycache.NewMemoryCacheProvider[int]("my-cache-key")
	
	// Set a value with a custom TTL.
	provider.Set(42, 10*time.Second)
	
	// Retrieve the cached value.
	value, err := provider.Get()
	if err != nil {
		fmt.Println("Error retrieving value:", err)
		return
	}
	fmt.Println("Cached value:", value)
}
```

Using MemoryCacheBroker

```
package main

import (
	"fmt"
	"time"

	memorycache "github.com/usuginus/go-cache-helper"
)

func main() {
	// Create a new MemoryCacheBroker with a specific key and TTL.
	broker := memorycache.NewMemoryCacheBroker[string]("my-broker-key", 30*time.Second)
	
	// Execute a data-fetching function through the cache.
	value, err := broker.Exec(func() (string, error) {
		// Simulate fetching data from an origin source (e.g., database or API).
		return "Hello, Cached World!", nil
	})
	if err != nil {
		fmt.Println("Error executing through cache:", err)
		return
	}
	
	fmt.Println("Result:", value)
	
	// Optionally clear the cache (useful for testing purposes).
	broker.ClearCache()
}
```
