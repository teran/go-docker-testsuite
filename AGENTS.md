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
   - Integration tests use real Docker containers (skipped if Docker is
     unavailable).
   - Write testable `Example*` functions for public APIs.
   - Versioned integration tests go in `applications/*/versions/`.

5. **No mocks**: Prefer real containers over mocks. The library exists to
   provide the highest possible test quality and accuracy without requiring
   manual infrastructure setup when running tests.

6. **Identifier validation**: Application wrappers that execute DDL
   (`CreateDB`, `CreateKeyspace`, etc.) **must** validate identifiers to
   prevent injection. Use `unicode.IsLetter`/`unicode.IsDigit` for Unicode
   portability, not `regexp`.

7. **Types**: Named types (`type ContainerID = string`) for documentation
   only — they are actual string aliases, not opaque types.

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
│   ├── kafka/
│   ├── memcache/
│   ├── minio/
│   ├── mysql/
│   ├── postgres/
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
