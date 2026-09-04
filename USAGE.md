# dix - Usage Documentation

Complete guide to using the dix dependency injection container.

## Table of Contents

1. [Getting Started](#getting-started)
2. [Core Concepts](#core-concepts)
3. [Registration Patterns](#registration-patterns)
4. [Resolution Patterns](#resolution-patterns)
5. [Scopes and Lifetimes](#scopes-and-lifetimes)
6. [Real-World Examples](#real-world-examples)
7. [Common Patterns](#common-patterns)
8. [Troubleshooting](#troubleshooting)

---

## Getting Started

### Installation

```bash
go get github.com/dix
```

### Minimal Example

```go
package main

import (
    "fmt"
    "github.com/dix"
)

type Greeter struct {
    message string
}

func main() {
    container := dix.New()
    
    container.Singleton(func() *Greeter {
        return &Greeter{message: "Hello, World!"}
    })
    
    var greeter *Greeter
    container.Resolve(&greeter)
    
    fmt.Println(greeter.message)
}
```

---

## Core Concepts

### Container

The container is the central registry for all dependencies. It manages:
- Registration of factories and instances
- Resolution of dependencies
- Creation of scopes

```go
container := dix.New()
```

### Resolver

A resolver can inject dependencies into target variables:

```go
var service *MyService
resolver.Resolve(&service)
```

### Scope

A scope provides a bounded lifetime for scoped dependencies:

```go
scope := container.NewScope()
defer scope.Close()

var service *ScopedService
scope.Resolve(&service)
```

---

## Registration Patterns

### 1. Singleton Registration

A singleton is created once and shared throughout the application lifetime.

```go
// Simple singleton
container.Singleton(func() *Config {
    return &Config{
        AppName: "MyApp",
        Version: "1.0.0",
    }
})

// Singleton with initialization
container.Singleton(func() *Database {
    db, err := sql.Open("postgres", "connection-string")
    if err != nil {
        panic(err)
    }
    return &Database{conn: db}
})
```

**Use cases:**
- Application configuration
- Database connections
- Logger instances
- Cache managers

### 2. Scoped Registration

Scoped dependencies are created once per scope.

```go
container.Scoped(func() *RequestContext {
    return &RequestContext{
        RequestID: uuid.New().String(),
        StartTime: time.Now(),
    }
})

container.Scoped(func(r dix.Resolver) *UnitOfWork {
    var db *Database
    if err := r.Resolve(&db); err != nil {
        panic(err)
    }
    return &UnitOfWork{db: db}
})
```

**Use cases:**
- HTTP request contexts
- Database transactions
- User sessions
- Request-specific caches

### 3. Transient Registration

Transient dependencies are created every time they're resolved.

```go
container.Transient(func() *EmailMessage {
    return &EmailMessage{
        Timestamp: time.Now(),
        ID:        uuid.New(),
    }
})

container.Transient(func(r dix.Resolver) *Command {
    var repo *Repository
    if err := r.Resolve(&repo); err != nil {
        panic(err)
    }
    return &Command{repo: repo}
})
```

**Use cases:**
- Stateful objects
- Command objects
- Unique identifiers
- Timestamps

### 4. Instance Registration

Register a pre-constructed object.

```go
logger := &Logger{
    level: LogLevelInfo,
    output: os.Stdout,
}
container.Instance(logger)

// Or with interfaces
var loggerInterface Logger = logger
container.Instance(loggerInterface)
```

**Use cases:**
- Pre-configured objects
- Test mocks
- External dependencies
- Third-party libraries

---

## Resolution Patterns

### 1. Basic Resolution

```go
var service *MyService
if err := container.Resolve(&service); err != nil {
    log.Fatal(err)
}
```

### 2. Interface Resolution

```go
type Logger interface {
    Log(message string)
}

type ConsoleLogger struct{}

func (c *ConsoleLogger) Log(message string) {
    fmt.Println(message)
}

// Register concrete type
logger := &ConsoleLogger{}
container.Instance(logger)

// Resolve by interface
var l Logger
container.Resolve(&l)
l.Log("Message")
```

If **two** registered types implement `Logger`, this fails with
`ErrAmbiguousResolution` rather than returning one of them — see the
troubleshooting entry for why. Register one, or resolve the concrete type.

### 3. Resolve All Dependencies

```go
var allServices []interface{}
container.ResolveAll(&allServices)

for _, svc := range allServices {
    fmt.Printf("Service: %T\n", svc)
}
```

On the root container this covers singletons and transients. `Scoped`
registrations are skipped, for the same reason `Resolve` refuses them — call
`ResolveAll` on a `Scope` to include them, and that scope will close what it
built.

### 4. Nested Resolution

```go
container.Singleton(func() *Database {
    return &Database{connectionString: "..."}
})

container.Singleton(func(r dix.Resolver) *UserRepository {
    var db *Database
    if err := r.Resolve(&db); err != nil {
        panic(err)
    }
    return &UserRepository{db: db}
})

container.Singleton(func(r dix.Resolver) *UserService {
    var repo *UserRepository
    if err := r.Resolve(&repo); err != nil {
        panic(err)
    }
    return &UserService{repo: repo}
})
```

---

## Scopes and Lifetimes

### Understanding Lifetimes

| Lifetime | Creation | Sharing | Cleanup |
|----------|----------|---------|---------|
| Singleton | Once per container | Shared globally | Container disposal |
| Scoped | Once per scope | Shared within scope | Scope.Close() |
| Transient | Every resolution | Never shared | Immediate |

### Working with Scopes

#### Creating a Scope

```go
scope := container.NewScope()
defer scope.Close()
```

#### Scope Isolation

```go
container.Scoped(func() *Session {
    return &Session{ID: uuid.New()}
})

// First scope
scope1 := container.NewScope()
var session1 *Session
scope1.Resolve(&session1)

// Second scope - different instance
scope2 := container.NewScope()
var session2 *Session
scope2.Resolve(&session2)

fmt.Println(session1 != session2) // true
```

#### Automatic Resource Cleanup

```go
type DbTransaction struct {
    tx *sql.Tx
}

func (d *DbTransaction) Close() error {
    return d.tx.Rollback()
}

container.Scoped(func(r dix.Resolver) *DbTransaction {
    var db *Database
    if err := r.Resolve(&db); err != nil {
        panic(err)
    }
    tx, _ := db.conn.Begin()
    return &DbTransaction{tx: tx}
})

scope := container.NewScope()
defer scope.Close() // Automatically calls Close() on DbTransaction
```

---

## Real-World Examples

### Example 1: Web Application

```go
package main

import (
    "encoding/json"
    "net/http"
    "github.com/dix"
)

// Domain models
type User struct {
    ID   int
    Name string
}

// Services
type Database struct {
    connectionString string
}

type UserRepository struct {
    db *Database
}

func (r *UserRepository) GetAll() []User {
    // Database logic
    return []User{{ID: 1, Name: "Alice"}}
}

type UserService struct {
    repo *UserRepository
}

func (s *UserService) ListUsers() []User {
    return s.repo.GetAll()
}

// Setup container
func setupContainer() dix.Container {
    container := dix.New()
    
    // Singleton database
    container.Singleton(func() *Database {
        return &Database{connectionString: "server=localhost"}
    })
    
    // Scoped repository
    container.Scoped(func(r dix.Resolver) *UserRepository {
        var db *Database
        if err := r.Resolve(&db); err != nil {
            panic(err)
        }
        return &UserRepository{db: db}
    })
    
    // Transient service.
    //
    // The Resolver argument is not optional here: UserService depends on the
    // Scoped UserRepository, so it has to resolve through whatever Resolver
    // asked for it. Closing over `container` would resolve the repository from
    // the root, which returns ErrScopedFromRoot.
    container.Transient(func(r dix.Resolver) *UserService {
        var repo *UserRepository
        if err := r.Resolve(&repo); err != nil {
            panic(err)
        }
        return &UserService{repo: repo}
    })
    
    return container
}

// HTTP handler
func makeUserHandler(container dix.Container) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // Create scope for this request
        scope := container.NewScope()
        defer scope.Close()
        
        // Resolve service
        var service *UserService
        if err := scope.Resolve(&service); err != nil {
            http.Error(w, err.Error(), http.StatusInternalServerError)
            return
        }
        
        // Use service
        users := service.ListUsers()
        json.NewEncoder(w).Encode(users)
    }
}

func main() {
    container := setupContainer()
    
    http.HandleFunc("/users", makeUserHandler(container))
    http.ListenAndServe(":8080", nil)
}
```

### Example 2: CLI Application

```go
package main

import (
    "flag"
    "fmt"
    "github.com/dix"
)

type Config struct {
    Verbose bool
}

type Logger struct {
    config *Config
}

func (l *Logger) Log(message string) {
    if l.config.Verbose {
        fmt.Println("[LOG]", message)
    }
}

type Application struct {
    logger  *Logger
    config  *Config
}

func (a *Application) Run() {
    a.logger.Log("Application started")
    fmt.Println("Running...")
}

func main() {
    verbose := flag.Bool("verbose", false, "Enable verbose logging")
    flag.Parse()
    
    container := dix.New()
    
    // Register config
    config := &Config{Verbose: *verbose}
    container.Instance(config)
    
    // Register logger
    container.Singleton(func() *Logger {
        var cfg *Config
        container.Resolve(&cfg)
        return &Logger{config: cfg}
    })
    
    // Register application
    container.Singleton(func() *Application {
        var logger *Logger
        var config *Config
        container.Resolve(&logger)
        container.Resolve(&config)
        return &Application{logger: logger, config: config}
    })
    
    // Run
    var app *Application
    container.Resolve(&app)
    app.Run()
}
```

### Example 3: Background Worker

```go
package main

import (
    "context"
    "fmt"
    "time"
    "github.com/dix"
)

type JobQueue interface {
    Next() (Job, error)
}

type Job struct {
    ID   string
    Data string
}

type Worker struct {
    queue  JobQueue
    logger *Logger
}

func (w *Worker) Process(ctx context.Context) {
    for {
        select {
        case <-ctx.Done():
            return
        default:
            job, err := w.queue.Next()
            if err != nil {
                w.logger.Log("Error getting job: " + err.Error())
                time.Sleep(time.Second)
                continue
            }
            w.processJob(job)
        }
    }
}

func (w *Worker) processJob(job Job) {
    w.logger.Log(fmt.Sprintf("Processing job %s", job.ID))
    // Process job
}

func main() {
    container := dix.New()
    
    // Singleton queue
    container.Singleton(func() JobQueue {
        return &InMemoryQueue{}
    })
    
    // Singleton logger
    container.Singleton(func() *Logger {
        return &Logger{}
    })
    
    // Transient workers
    container.Transient(func() *Worker {
        var queue JobQueue
        var logger *Logger
        container.Resolve(&queue)
        container.Resolve(&logger)
        return &Worker{queue: queue, logger: logger}
    })
    
    // Start workers
    ctx := context.Background()
    for i := 0; i < 5; i++ {
        var worker *Worker
        container.Resolve(&worker)
        go worker.Process(ctx)
    }
    
    select {} // Keep running
}
```

---

## Common Patterns

### Pattern 1: Factory with Configuration

```go
type ServiceConfig struct {
    Timeout time.Duration
    Retries int
}

container.Instance(&ServiceConfig{
    Timeout: 30 * time.Second,
    Retries: 3,
})

container.Singleton(func() *Service {
    var config *ServiceConfig
    container.Resolve(&config)
    return NewService(config)
})
```

### Pattern 2: Conditional Registration

```go
func registerServices(container dix.Container, env string) {
    if env == "production" {
        container.Singleton(func() Logger {
            return &FileLogger{path: "/var/log/app.log"}
        })
    } else {
        container.Singleton(func() Logger {
            return &ConsoleLogger{}
        })
    }
}
```

### Pattern 3: Testing with Mocks

```go
func TestUserService(t *testing.T) {
    container := dix.New()
    
    // Register mock. Depend on an interface, not the concrete repository —
    // a mock is a different type, so a factory asking for *UserRepository
    // will not find *MockUserRepository.
    mockRepo := &MockUserRepository{
        users: []User{{ID: 1, Name: "Test"}},
    }
    container.Instance(mockRepo)
    
    // Register service
    container.Singleton(func(r dix.Resolver) *UserService {
        var repo UserRepository // interface
        if err := r.Resolve(&repo); err != nil {
            t.Fatal(err)
        }
        return &UserService{repo: repo}
    })
    
    // Test
    var service *UserService
    container.Resolve(&service)
    
    users := service.ListUsers()
    if len(users) != 1 {
        t.Errorf("Expected 1 user, got %d", len(users))
    }
}
```

### Pattern 4: Middleware Pattern

```go
type Middleware func(http.Handler) http.Handler

func LoggingMiddleware(container dix.Container) Middleware {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            scope := container.NewScope()
            defer scope.Close()
            
            var logger *Logger
            scope.Resolve(&logger)
            
            logger.Log(fmt.Sprintf("%s %s", r.Method, r.URL.Path))
            next.ServeHTTP(w, r)
        })
    }
}
```

---

## Troubleshooting

### `ErrNotRegistered`

**Problem:** Trying to resolve a type that hasn't been registered.

**Solution:**
```go
// Wrong
var service *MyService
container.Resolve(&service) // Error!

// Correct
container.Singleton(func() *MyService {
    return &MyService{}
})
var service *MyService
container.Resolve(&service) // OK
```

### `ErrInvalidTarget`

**Problem:** Passing a non-pointer value to `Resolve()`.

**Solution:**
```go
// Wrong
var service MyService
container.Resolve(service) // Error!

// Correct
var service *MyService
container.Resolve(&service) // OK
```

### `ErrInvalidFactory`

**Problem:** The factory signature is not one of the two accepted forms.

**Solution:**
```go
// Wrong — two return values
container.Singleton(func() (*MyService, error) {
    return &MyService{}, nil
})

// Wrong — an argument that is not a dix.Resolver
container.Singleton(func(cfg *Config) *MyService {
    return &MyService{cfg: cfg}
})

// Correct — no arguments
container.Singleton(func() *MyService {
    return &MyService{}
})

// Correct — dependencies via the Resolver
container.Singleton(func(r dix.Resolver) *MyService {
    var cfg *Config
    if err := r.Resolve(&cfg); err != nil {
        panic(err)
    }
    return &MyService{cfg: cfg}
})
```

### `ErrScopedFromRoot`

**Problem:** A `Scoped` type was resolved from the root container.

The container refuses rather than obliging, because a scoped value built from
the root has no owning scope — nothing would ever call its `Close`, so every
resolve would leak it.

**Solution:** resolve it from a scope, and let dependent factories take a
`Resolver` so they inherit the caller's scope:
```go
container.Scoped(func() *RequestContext { return &RequestContext{} })

// Wrong
var rc *RequestContext
container.Resolve(&rc) // ErrScopedFromRoot

// Correct
scope := container.NewScope()
defer scope.Close()
var rc *RequestContext
scope.Resolve(&rc) // OK

// Correct — a dependent factory inherits whatever Resolver asked for it
container.Transient(func(r dix.Resolver) *Handler {
    var rc *RequestContext
    if err := r.Resolve(&rc); err != nil {
        panic(err)
    }
    return &Handler{rc: rc}
})
```

### `ErrAmbiguousResolution`

**Problem:** Two or more registered types satisfy the interface being resolved.

Picking one would mean picking by Go's randomised map iteration order, so the
same binary would resolve a different implementation on each start. The error
names every candidate.

**Solution:** register one implementation, or resolve the concrete type:
```go
container.Instance(&FileLogger{})
container.Instance(&ConsoleLogger{})

// Wrong
var l Logger
container.Resolve(&l) // ErrAmbiguousResolution: *FileLogger, *ConsoleLogger

// Correct — ask for what you actually want
var l *FileLogger
container.Resolve(&l) // OK
```

### `ErrAlreadyRegistered`

**Problem:** The same type was registered twice.

**Solution:** pick one lifetime. A type may hold only one registration, because
two would make the effective lifetime depend on lookup precedence rather than
on anything the author chose.

### `ErrScopeClosed`

**Problem:** A scope was used after `Close`. Usually a `defer scope.Close()`
in an outer function while an inner goroutine still holds the scope.

**Solution:** keep the scope alive until every goroutine using it has finished,
or give each goroutine its own scope.

### Circular Dependencies

**Problem:** Service A depends on Service B, which depends on Service A.

**Solution:** Restructure your dependencies or use lazy initialization:

```go
type ServiceA struct {
    getB func() *ServiceB
}

type ServiceB struct {
    a *ServiceA
}

container.Singleton(func(r dix.Resolver) *ServiceA {
    return &ServiceA{
        // Deferred: resolved on first call, not during construction, so the
        // cycle is broken in time rather than in structure.
        getB: func() *ServiceB {
            var b *ServiceB
            if err := r.Resolve(&b); err != nil {
                panic(err)
            }
            return b
        },
    }
})

container.Singleton(func(r dix.Resolver) *ServiceB {
    var a *ServiceA
    if err := r.Resolve(&a); err != nil {
        panic(err)
    }
    return &ServiceB{a: a}
})
```

### Memory Leaks in Scopes

**Problem:** Forgetting to close scopes.

**Solution:** Always use `defer`:

```go
// Wrong
scope := container.NewScope()
// ... use scope
scope.Close() // Might not be called if panic occurs

// Correct
scope := container.NewScope()
defer scope.Close() // Always called
// ... use scope
```

---

## Best Practices

1. **Register interfaces, resolve by interface**: Promotes loose coupling
2. **Use appropriate lifetimes**: Choose the right lifetime for each dependency
3. **Always defer scope.Close()**: Ensure resources are cleaned up
4. **Keep factories simple**: Complex logic belongs in constructors, not factories
5. **Avoid the service locator anti-pattern**: Pass dependencies explicitly when possible
6. **Test with separate containers**: Create isolated containers for tests
7. **Document dependencies**: Make it clear what each service depends on

---

## Advanced Topics

### Custom Resolver Implementation

You can implement your own resolver:

```go
type customResolver struct {
    values map[reflect.Type]any
}

func (r *customResolver) Resolve(target any) error {
    // Custom resolution logic
    return nil
}

func (r *customResolver) ResolveAll(target any) error {
    // Custom resolution logic
    return nil
}
```

### Integration with Other Libraries

```go
// With gorilla/mux
func setupRouter(container dix.Container) *mux.Router {
    r := mux.NewRouter()
    r.HandleFunc("/users", makeUserHandler(container))
    return r
}

// With chi
func setupChi(container dix.Container) chi.Router {
    r := chi.NewRouter()
    r.Use(LoggingMiddleware(container))
    r.Get("/users", makeUserHandler(container))
    return r
}
```

---

## Performance Tips

1. **Prefer singletons for expensive objects**: Database connections, HTTP clients
2. **Use transient sparingly**: Only when necessary for stateful objects
3. **Minimize resolution calls**: Resolve once and reuse when possible
4. **Benchmark critical paths**: Use Go's benchmarking tools

```go
func BenchmarkResolve(b *testing.B) {
    container := dix.New()
    container.Singleton(func() *MyService {
        return &MyService{}
    })
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        var service *MyService
        container.Resolve(&service)
    }
}
```

---

For more examples and updates, visit the [GitHub repository](https://github.com/kryovyx/dix).
