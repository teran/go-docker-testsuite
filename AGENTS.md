# Agent Instructions for go-docker-testsuite

## Project identity

You are working on **go-docker-testsuite** — a Go library that spins up
third-party Docker containers for integration testing. The module path is
`github.com/teran/go-docker-testsuite`, requires Go 1.26+, and uses the
official Docker SDK (no CLI shell-outs).

## Language

All communication (code, comments, commit messages, documentation, and
discussion) **must be in English**. The project is English-only because
maintainers may not speak other languages — requests or contributions in
other languages cannot be accepted.

## Code conventions

1. **Go style**: Follow `gofmt` and `golangci-lint` (staticcheck enabled).
   Run `golangci-lint run ./...` before committing.

2. **Error handling**: Wrap errors with `github.com/pkg/errors` (`errors.Wrap`,
   `errors.Errorf`). Never use `fmt.Errorf` for wrapped errors in core
   packages; `fmt.Errorf` is acceptable in application packages that don't
   import `pkg/errors`.

3. **Logging**: Use `github.com/sirupsen/logrus`. Library-internal messages
   use `Trace`/`Debug` level. Let the application layer decide log severity.

4. **Testing**:
   - Integration tests use real Docker containers.
   - Write testable `Example*` functions for public APIs.
   - Versioned integration tests go in `applications/*/versions/`.

5. **No mocks**: Prefer real containers over mocks. The library exists to
   provide the highest possible test quality and accuracy without requiring
   manual infrastructure setup when running tests.

6. **Identifier validation**: Application wrappers that execute DDL
   (`CreateDB`, `CreateKeyspace`, etc.) **must** validate identifiers to
   prevent injection. Use a strict **printable-ASCII whitelist**
   (`[a-zA-Z0-9_]`, plus any service-specific extra characters such as `-`
   for MongoDB or `$` for MySQL) plus a length cap, never `regexp` and never
   a permissive `unicode.IsLetter`/`unicode.IsDigit` allow-list. An
   ASCII whitelist excludes Unicode normalization / homoglyph attacks, which
   are a real risk for database identifiers.

7. **Types**: Named types (`type ContainerID = string`) for documentation
   only — they are actual string aliases, not opaque types.

8. **File seeding (`WithFiles`)**: When a container needs configuration or
   seed files, prefer `WithFiles` (a `LifecycleOption` via
   `NewContainerWithLifecycle`) over shelling out. `File` streams `Content`
   from an `io.Reader` with a **required exact `Size`** so files larger than
   RAM are copied without buffering; use the `FileFromBytes` helper for small
   in-memory content. Always validate `Destination` (absolute path, no `..`).

9. **Application constructors**: Every application package exposes the
   standard four-constructor surface (the PostgreSQL pattern):
   `New(ctx)`, `NewWithImage(ctx, image)`, `NewWithT(t, ctx)`,
   `NewWithImageT(t, ctx, image)` — where the `New*` default uses the
   `images.X` constant and the `*T` variants bind the lifecycle to the test
   via `t.Cleanup`. Wrappers needing extra args add trailing parameters
   (`opts ...Option`, config, etc.). New wrappers **must** follow this
   contract. Do **not** introduce a new `New(ctx, image)` single-argument
   form without a default image; the pre-existing `mysql`/`redis`/`vault`
   packages keep their legacy signatures for backward compatibility.

10. **Multi-module & release order**: The repository is a multi-module
    workspace (testcontainers-go style): the root is the core module and each
    `applications/<name>` is its own Go module with its own `go.mod`.
    Applications are leaf nodes — they import the core (`wait`/`images`/
    `internal`) and never other applications. During development an
    application `go.mod` may carry `replace github.com/teran/go-docker-testsuite
    => ../..` so a checkout builds before the core is published; this
    `replace` **must be removed before release** (`make lint` fails if any
    `go.mod` has a `replace`). Release each module in its own PR, and **always
    release/tag the core before any application** that depends on the new core
    version (applications `require` the core at a concrete version).
    Tag scheme: core `v<version>`, applications `applications/<name>/v<version>`
    on the same commit via `make tag <version>`.

## Project structure

```text
./
├── application.go          # Application (hooks wrapper)
├── container.go            # Container implementation
├── environment.go          # Fluent env-var builder
├── group.go                # Multi-container network
├── matcher.go              # Log matchers (substr, exact, regexp)
├── ports.go                # Port binding configuration
├── protocol.go             # TCP/UDP protocol type
├── node.go                 # Node info (Docker host IP)
├── container_info.go       # ContainerInfo interface
├── images/images.go        # Well-known image references
├── internal/               # Internal helpers (ptr, random)
├── applications/           # Typed service wrappers
│   ├── ceph/
│   ├── k3s/
│   ├── kafka/
│   ├── memcache/
│   ├── minio/
│   ├── mysql/
│   ├── opensearch/
│   ├── postgres/
│   ├── rabbitmq/
│   ├── redis/
│   ├── scylladb/
│   └── vault/
└── SPEC.md                 # Full architecture specification
```

## When agents should ask

- If a change would introduce a new dependency — ask first.
- If a change would break the `Container` interface — ask first (it affects
  all application packages).
- If you're unsure about identifier validation rules for a new application
  wrapper — ask.

## Commit messages

Write concise, descriptive commit messages in English. Start with a capital
letter and keep the first line under 72 characters. No rigid format required.

## AI-assisted development

Commits authored or assisted by AI agents are welcome, with the following
conditions:

- **The human author bears full responsibility** for the commit's correctness,
  safety, and adherence to project conventions — just as with any other commit.
- **The human author is responsible** for iterating on feedback, fixing issues,
  and obtaining approval in a pull request. AI agents cannot fulfill this role.
- **AI opinions are not arguments** in discussions. During code review, only the
  human author's expertise and reasoning count — citing an AI's suggestion does
  not carry weight.
- Using AI does **not lower the bar** for the author's required expertise in the
  change they are making.
