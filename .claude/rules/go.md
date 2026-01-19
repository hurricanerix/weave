---
paths:
  - "**/*.go"
  - "**/go.mod"
  - "**/go.sum"
---

# Go Language Rules for Weave

## Philosophy

Write Go code that looks like it belongs in the standard library. If it doesn't, it's wrong.

**Priorities:**
1. Correctness
2. Clarity  
3. Simplicity
4. Performance (in that order)

Boring code is beautiful code.

## Language Standards

### Version
- **Go 1.22+** minimum
- Use latest stable Go features
- No experimental features in production code

### Project Layout
Follow standard Go project layout:
```
weave/
├── backend/            # Go backend service
│   ├── cmd/weave/      # Main application
│   ├── internal/       # Private packages
│   │   ├── protocol/   # Binary protocol
│   │   ├── scheduler/  # Job scheduling
│   │   ├── web/        # HTTP server
│   │   └── client/     # Compute client
│   ├── pkg/            # Public libraries (if any)
│   ├── test/integration/ # Integration tests
│   ├── go.mod
│   └── go.sum
└── compute/            # C GPU compute component
```

## Code Style

### Formatting
- `gofmt` or `goimports` - NO exceptions
- Use `make fmt` before committing
- Line length: Keep reasonable (<120 chars preferred)

### Naming
- **Packages**: lowercase, single word, no underscores
  - ✅ `scheduler`, `protocol`, `client`
  - ❌ `job_scheduler`, `protocolHandler`, `compute_client`

- **Interfaces**: -er suffix for single-method interfaces
  - ✅ `Reader`, `Writer`, `Scheduler`
  - ❌ `IReader`, `ReadInterface`

- **Variables**: 
  - Short names for short scopes: `i`, `err`, `ctx`
  - Descriptive names for package scope: `defaultTimeout`, `maxRetries`
  - NO Hungarian notation

### Error Handling

**Errors are values**, not exceptions.

```go
// ✅ GOOD - Check errors immediately
result, err := compute.Generate(prompt)
if err != nil {
    return fmt.Errorf("generation failed: %w", err)
}

// ❌ BAD - Ignoring errors
result, _ := compute.Generate(prompt)

// ❌ BAD - Panic in library code
if err != nil {
    panic(err)  // Only in main() or tests
}
```

**Error wrapping:**
- Use `fmt.Errorf("context: %w", err)` for wrapping
- Use `errors.Is()` and `errors.As()` for checking
- Define sentinel errors: `var ErrSocketClosed = errors.New("socket closed")`

### Context

**Always pass context.Context as first parameter:**

```go
// ✅ GOOD
func Generate(ctx context.Context, prompt string) (*Image, error)

// ❌ BAD  
func Generate(prompt string) (*Image, error)
```

Respect context cancellation:
```go
select {
case result := <-resultCh:
    return result, nil
case <-ctx.Done():
    return nil, ctx.Err()
}
```

## Testing

### Unit Tests

**Table-driven tests are mandatory** for functions with multiple cases:

```go
func TestValidatePrompt(t *testing.T) {
    tests := []struct {
        name    string
        prompt  string
        wantErr bool
    }{
        {"valid prompt", "a cat in space", false},
        {"empty prompt", "", true},
        {"too long", strings.Repeat("a", 10000), true},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := ValidatePrompt(tt.prompt)
            if (err != nil) != tt.wantErr {
                t.Errorf("got error %v, wantErr %v", err, tt.wantErr)
            }
        })
    }
}
```

**Unit test requirements:**
- **Fast**: Run in <100ms total
- **Isolated**: No I/O (filesystem, network, GPU)
- **Deterministic**: Same input = same output
- **One assertion per test** (use sub-tests if needed)

### Integration Tests

For tests that touch real resources:

```go
//go:build integration

package client_test

func TestRealSocket(t *testing.T) {
    // This touches real Unix socket
}
```

Run with: `go test -tags=integration`

### Test Helpers

Use `t.Helper()` in test utilities:

```go
func assertNoError(t *testing.T, err error) {
    t.Helper()
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
}
```

### Test Coverage

- Aim for >80% coverage on critical paths
- Use `go test -cover`
- Don't chase 100% (diminishing returns)

## Concurrency

### Goroutine Management

**Never leak goroutines:**

```go
// ✅ GOOD - Goroutine stops when context is cancelled
func worker(ctx context.Context) {
    for {
        select {
        case <-ctx.Done():
            return
        case work := <-workCh:
            process(work)
        }
    }
}

// ❌ BAD - Goroutine never stops
func worker() {
    for work := range workCh {
        process(work)
    }
}
```

### Channels

- Sender closes channels, not receiver
- Buffered channels when you know the size
- Unbuffered for synchronization

```go
// ✅ GOOD
results := make(chan Result, 10)
go func() {
    defer close(results)  // Sender closes
    // ... send results
}()

// ❌ BAD
results := make(chan Result)
close(results)  // Receiver closing
```

### sync Package

- Use `sync.Mutex` for protecting shared state
- Use `sync.RWMutex` when reads >> writes
- Use `sync.WaitGroup` for waiting on goroutines
- Use `sync.Once` for one-time initialization

## Performance

### Allocations

Minimize allocations in hot paths:

```go
// ✅ GOOD - Reuse buffer
var buf bytes.Buffer
buf.Reset()
buf.WriteString(data)

// ❌ BAD - New allocation every time
s := data1 + data2 + data3
```

### Profiling

Use built-in profiling:
```bash
go test -bench=. -cpuprofile=cpu.prof
go tool pprof cpu.prof
```

### Benchmarks

Write benchmarks for performance-critical code:

```go
func BenchmarkProtocolEncode(b *testing.B) {
    req := &Request{Prompt: "test"}
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _ = protocol.Encode(req)
    }
}
```

Run with: `go test -bench=.`

## Dependencies

### Module Management
- Use `go mod tidy` to clean dependencies
- Pin versions in `go.mod`
- Vendor critical dependencies if needed

### Choosing Dependencies
**Be conservative**. Every dependency is:
- A security risk
- A maintenance burden  
- A compatibility constraint

**Ask:**
- Can I write this in <100 lines?
- Is this dependency maintained?
- Does it have CVEs?
- Do I really need it?

**Prefer stdlib** when possible.

## Common Patterns

### Options Pattern

For complex constructors:

```go
type Option func(*Client)

func WithTimeout(d time.Duration) Option {
    return func(c *Client) {
        c.timeout = d
    }
}

func NewClient(addr string, opts ...Option) *Client {
    c := &Client{addr: addr, timeout: 30 * time.Second}
    for _, opt := range opts {
        opt(c)
    }
    return c
}
```

### Interfaces

**Accept interfaces, return structs:**

```go
// ✅ GOOD
func Process(r io.Reader) (*Result, error)

// ❌ BAD
func Process(r io.Reader) (io.Reader, error)
```

Keep interfaces small:
```go
// ✅ GOOD - Single method
type Generator interface {
    Generate(context.Context, string) (*Image, error)
}

// ❌ BAD - God interface
type Service interface {
    Generate(...) (...)
    Validate(...) (...)
    Cache(...) (...)
    // ... 10 more methods
}
```

## Documentation

### Package Comments

Every package needs a doc comment:

```go
// Package protocol implements the binary protocol for
// communication between weave-backend and weave-compute.
package protocol
```

### Function Comments

Document exported functions:

```go
// Generate creates an image from the given prompt.
// It returns ErrInvalidPrompt if the prompt is empty or too long.
func Generate(ctx context.Context, prompt string) (*Image, error)
```

### Examples

Provide examples for complex APIs:

```go
func ExampleClient_Generate() {
    client := NewClient("/run/weave/compute.sock")
    img, err := client.Generate(context.Background(), "a cat")
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(img.Width, img.Height)
    // Output: 512 512
}
```

## Temporary Files

**Always use `./tmp/` (project-local), never `/tmp/`.**

```go
// ✅ GOOD - Project-local temp directory
if err := os.MkdirAll("./tmp", 0755); err != nil {
    return fmt.Errorf("failed to create tmp dir: %w", err)
}
f, err := os.CreateTemp("./tmp", "weave-*.tmp")

// ❌ BAD - System temp directory
f, err := os.CreateTemp("", "weave-*.tmp")  // Uses /tmp
f, err := os.CreateTemp("/tmp", "weave-*")  // Explicit /tmp
```

**Why:**
- Keeps test artifacts contained to the project
- Easier cleanup
- Avoids permission issues in sandboxed environments (Flatpak)
- Project `.gitignore` already ignores `./tmp/`

## Anti-Patterns to Avoid

❌ **Panic in library code** - Return errors instead  
❌ **Global state** - Pass dependencies explicitly  
❌ **init() functions** - Use explicit initialization  
❌ **Reflection** - Use it only when absolutely necessary  
❌ **Empty interface{}** - Use concrete types or generics  
❌ **Context.Value** - Use it only for request-scoped values  
❌ **Premature optimization** - Profile first

## Build and Tools

### Makefile

Standard targets:
```makefile
.PHONY: test
test:
    cd backend && go test -v ./...

.PHONY: test-integration
test-integration:
    cd backend && go test -v -tags=integration ./...

.PHONY: bench
bench:
    cd backend && go test -bench=. -benchmem ./...

.PHONY: fmt
fmt:
    cd backend && go fmt ./...
    cd backend && goimports -w .

.PHONY: lint
lint:
    cd backend && golangci-lint run

.PHONY: build
build:
    cd backend && go build -o ../bin/weave ./cmd/weave
```

### CI Checks

Must pass:
- `cd backend && go test ./...`
- `cd backend && go vet ./...`
- `cd backend && golangci-lint run`
- `cd backend && go mod tidy` (no changes)

## When in Doubt

1. Check the Go standard library
2. Read Effective Go
3. Look at well-written Go projects (Docker, Kubernetes, CockroachDB)
4. Ask: "Would this pass code review at Google?"

If you wouldn't want to maintain this code in 2 years, rewrite it.
