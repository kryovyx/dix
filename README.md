# Dependency Injection eXperience (DIX)

A lightweight, reflection-based dependency injection container for Go.

[![Go Version](https://img.shields.io/badge/go-1.27+-blue.svg)](https://golang.org/dl/)
[![Coverage](https://img.shields.io/badge/coverage-94.2%25-brightgreen.svg)](#)
[![License](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

## Overview

`dix` is a minimal dependency injection (DI) container library for Go that provides:

- **Multiple lifetimes** — Singleton, Scoped, and Transient registrations
- **Direct instance registration** — Register pre-constructed values without a factory
- **Interface-based design** — Clean abstractions via `Container`, `Resolver`, and `Scope`
- **Reflection-based resolution** — Automatic dependency wiring using Go reflection
- **Scope management** — Create child scopes with isolated lifetimes and automatic cleanup
- **Resource cleanup** — Scoped instances implementing `io.Closer` are closed when the scope ends
- **100 % test coverage**

## Installation

```bash
go get github.com/kryovyx/dix
```

## Quick Start

```go
package main

import (
    "fmt"

    "github.com/kryovyx/dix"
)

// Services
type Database struct{ DSN string }
type UserRepo struct{ DB *Database }
type UserService struct{ Repo *UserRepo }

func main() {
    c := dix.New()

    // Singleton — one instance for the lifetime of the container
    c.Singleton(func() *Database {
        return &Database{DSN: "postgres://localhost/mydb"}
    })

    // Scoped — one instance per scope
    c.Scoped(func() *UserRepo {
        var db *Database
        c.Resolve(&db)
        return &UserRepo{DB: db}
    })

    // Transient — new instance on every resolve
    c.Transient(func() *UserService {
        var repo *UserRepo
        c.Resolve(&repo)
        return &UserService{Repo: repo}
    })

    // Instance — register a pre-constructed value
    c.Instance(&Database{DSN: "sqlite://memory"})

    // Resolve
    var svc *UserService
    if err := c.Resolve(&svc); err != nil {
        panic(err)
    }
    fmt.Printf("Service resolved: %+v\n", svc)
}
```

## Core Concepts

### Lifetimes

| Lifetime | Created | Shared |
|-----------|---------|--------|
| **Singleton** | Once, on first resolve | Across the entire container |
| **Scoped** | Once per scope | Within the same scope |
| **Transient** | Every resolve call | Never |
| **Instance** | By the caller (pre-built) | Across the entire container |

### Container

`Container` is the top-level interface. It extends `Resolver` with registration methods and scope creation.

```go
type Container interface {
    Resolver

    Singleton(factory any) error
    Scoped(factory any) error
    Transient(factory any) error
    Instance(v any) error
    NewScope() Scope
}
```

Create a container with `dix.New()`.

### Resolver

`Resolver` handles dependency look-up.

```go
type Resolver interface {
    Resolve(target any) error
    ResolveAll(target any) error
}
```

- `Resolve` — injects a single dependency into the target pointer.
- `ResolveAll` — injects all registered dependencies into a target slice pointer.

### Scope

`Scope` extends `Resolver` with a `Close` method. When a scope is closed every scoped instance that implements `io.Closer` (or has a `Close() error` method) is cleaned up automatically.

```go
type Scope interface {
    Resolver
    Close() error
}
```

### Factory Functions

A factory returns **exactly one value** and takes either **no arguments** or a
single **`dix.Resolver`**:

```go
// No dependencies.
container.Singleton(func() *MyService {
    return &MyService{}
})

// Dependencies resolved from the container (or the scope, for Scoped).
container.Singleton(func(r dix.Resolver) *MyService {
    var db *Database
    if err := r.Resolve(&db); err != nil {
        panic(err) // or return a service that reports the failure
    }
    return &MyService{DB: db}
})
```

Any other signature is rejected by the registration call itself with
`ErrInvalidFactory`, rather than panicking inside `reflect.Call` on the first
resolve.

A `Scoped` factory taking a `Resolver` receives **the scope**, not the root
container, so its own scoped dependencies stay inside the same scope.

### Errors

Every failure mode is a sentinel, so callers branch with `errors.Is` instead of
matching on message text:

| Error | Meaning |
|---|---|
| `ErrNotRegistered` | Nothing satisfies the requested type |
| `ErrScopedFromRoot` | A `Scoped` type was resolved from the root container — resolve it from a `Scope` |
| `ErrAmbiguousResolution` | More than one registration satisfies the requested interface; the message names them all |
| `ErrAlreadyRegistered` | The type already has a registration |
| `ErrInvalidFactory` | Unsupported factory signature, or `nil`/function passed to `Instance` |
| `ErrInvalidTarget` | `Resolve` target is not a pointer, or `ResolveAll` target is not a pointer to a slice of interfaces |
| `ErrScopeClosed` | The `Scope` was used after `Close` |

### One registration per type

A given type may be registered **once**, under one lifetime. Registering it
again — even under a different lifetime — returns `ErrAlreadyRegistered`.

Allowing two would force the container to choose between them by lookup
precedence, which makes the lifetime of a dependency depend on an ordering
nobody wrote down.

### Resolving interfaces

Requesting an interface matches any registered concrete type implementing it.
If **more than one** does, resolution fails with `ErrAmbiguousResolution` and
the error names every candidate.

This is deliberate. Go randomises map iteration order, so returning "the first
match" would resolve a different implementation on each process start — with
two loggers registered, which one a binary used would change from boot to boot.
Register one, or resolve the concrete type.

### Concurrency

`Container` and `Scope` are safe for concurrent use: registration and
resolution may run on different goroutines at the same time. Factories are
always invoked with the container's lock released, so a factory may resolve
further dependencies.

## API Reference

### `dix.New() Container`

Creates and returns a new container.

---

### `Container.Singleton(factory any) error`

Registers a factory whose return value is instantiated **once, on first
resolve**, and shared globally.

| Parameter | Description |
|-----------|-------------|
| `factory` | `func() T` or `func(dix.Resolver) T` |

Construction is lazy and happens exactly once, even under concurrent resolves.
A singleton factory may therefore depend on anything registered before the
first resolve, not merely on what was registered before it.

Returns `ErrInvalidFactory` for an unsupported signature, `ErrAlreadyRegistered`
if the returned type is already registered.

---

### `Container.Scoped(factory any) error`

Registers a factory whose return value is instantiated once per scope.

| Parameter | Description |
|-----------|-------------|
| `factory` | `func() T` or `func(dix.Resolver) T` — the `Resolver` is the scope |

Resolving a scoped type from the **root** container returns
`ErrScopedFromRoot` and constructs nothing: a scoped value built from the root
would have no owning scope, so nothing would ever `Close` it.

---

### `Container.Transient(factory any) error`

Registers a factory that produces a new instance on every `Resolve` call.

| Parameter | Description |
|-----------|-------------|
| `factory` | `func() T` or `func(dix.Resolver) T` |

---

### `Container.Instance(v any) error`

Registers a pre-constructed value. `v` must not be `nil` or a function.

| Parameter | Description |
|-----------|-------------|
| `v` | A non-nil, non-function value |

---

### `Container.NewScope() Scope`

Returns a new child `Scope`. Scoped registrations produce a fresh instance inside each scope.

---

### `Resolver.Resolve(target any) error`

Sets `*target` to the resolved dependency. `target` must be a pointer.

---

### `Resolver.ResolveAll(target any) error`

Appends every resolvable dependency to `*target`, which must be a pointer to a
slice of interfaces.

Called on the **root container** it covers singletons and transients only —
scoped registrations are skipped, for the same reason `Resolve` refuses them.
Called on a **`Scope`** it covers all three, and any scoped value it builds is
tracked by that scope and released by `Close`.

---

### `Scope.Close() error`

Releases all scoped resources. Calls `Close()` on every scoped instance that
has a `Close() error` method.

`Close` is **idempotent** — a `defer scope.Close()` alongside an explicit call
closes each instance once. Every closer is closed even if an earlier one
fails; the errors are joined with `errors.Join`. After `Close`, the scope
returns `ErrScopeClosed`.

## Examples

### Working with Scopes

```go
container := dix.New()

container.Scoped(func() *RequestCtx {
    return &RequestCtx{ID: generateID()}
})

// Scope 1
scope1 := container.NewScope()
defer scope1.Close()

var a, b *RequestCtx
scope1.Resolve(&a)
scope1.Resolve(&b)
fmt.Println(a == b) // true — same scope

// Scope 2
scope2 := container.NewScope()
defer scope2.Close()

var c *RequestCtx
scope2.Resolve(&c)
fmt.Println(a == c) // false — different scope
```

### Interface Binding

```go
type Logger interface { Log(msg string) }

type ConsoleLogger struct{}
func (l *ConsoleLogger) Log(msg string) { fmt.Println(msg) }

container := dix.New()
container.Instance(&ConsoleLogger{})

var log Logger
container.Resolve(&log)
log.Log("resolved via interface")
```

### Automatic Cleanup

```go
type DBConn struct{ pool *sql.DB }

func (d *DBConn) Close() error { return d.pool.Close() }

container := dix.New()
container.Scoped(func() *DBConn {
    pool, _ := sql.Open("postgres", "...")
    return &DBConn{pool: pool}
})

scope := container.NewScope()
var conn *DBConn
scope.Resolve(&conn)
// ... use conn ...
scope.Close() // conn.Close() called automatically
```

### Request-Scoped HTTP Handler

```go
func handler(container dix.Container) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        scope := container.NewScope()
        defer scope.Close()

        var svc *UserService
        scope.Resolve(&svc)
        json.NewEncoder(w).Encode(svc.ListUsers())
    }
}
```

### Testing with Mocks

```go
func TestUserRepo(t *testing.T) {
    c := dix.New()
    c.Instance(&MockDatabase{})

    c.Transient(func() *UserRepo {
        var db *Database
        c.Resolve(&db)
        return &UserRepo{DB: db}
    })

    var repo *UserRepo
    c.Resolve(&repo)
    // assert on repo
}
```

## Best Practices

1. **Pick the right lifetime** — Singleton for stateless/shared services, Scoped for request-bound resources, Transient for short-lived or stateful objects.
2. **Resolve by interface** — Register concrete types but resolve through interfaces to keep consumers decoupled.
3. **Always close scopes** — Use `defer scope.Close()` immediately after `NewScope()` to avoid resource leaks.
4. **Avoid circular dependencies** — The container does not detect cycles; a circular chain will loop forever.
5. **Keep factories simple** — A factory should construct and return; move complex logic into the service itself.
6. **Isolate tests** — Create a fresh container per test with mock implementations.

## Limitations

- **No automatic constructor injection** — Dependencies must be resolved manually inside factory functions.
- **No circular dependency detection** — Circular resolve chains will cause infinite recursion.
- **Type-based resolution only** — Resolution is keyed on Go types, not names or tags.
- **Single registration per type** — Registering the same type twice overwrites the previous registration.

## Contributing

**At this time, this project is in active development and is not open for external contributions.** The framework is still being refined and major interfaces may change.

Once the framework reaches a stable architecture and API, contributions from the community will be welcome. Please check back later or open an issue if you have feature requests or feedback.

## License

This project is licensed under the MIT License — see the [LICENSE](LICENSE) file for details.

## Copyright

© 2026 Kryovyx
