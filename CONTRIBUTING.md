# Contributing to go-docker-testsuite

Thank you for considering contributing! This document outlines the process.

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Getting Started](#getting-started)
- [Development Workflow](#development-workflow)
- [Adding a New Application Wrapper](#adding-a-new-application-wrapper)
- [Testing](#testing)
- [Pull Request Process](#pull-request-process)

## Code of Conduct

This project and everyone participating in it is governed by the
[CODE_OF_CONDUCT.md](./CODE_OF_CONDUCT.md). By participating, you are
expected to uphold this code.

## Getting Started

1. **Fork** the repository on GitHub.
2. **Clone** your fork:

   ```sh
   git clone https://github.com/<your-username>/go-docker-testsuite.git
   ```

3. **Ensure prerequisites**:
   - Go 1.26+
   - A running Docker daemon (required for integration tests)
4. **Run the linter** before making changes:

   ```sh
   golangci-lint run ./...
   ```

## Development Workflow

1. Create a feature branch from `master`:

   ```sh
   git checkout -b feat/my-feature
   ```

2. Make your changes.
3. Run `golangci-lint run ./...` — all lints must pass.
4. Run tests (see [Testing](#testing) below).
5. Commit following the [commit message](#commit-messages) guidelines.
6. Push and open a Pull Request.

### Commit Messages

Write concise, descriptive commit messages in English. Start with a capital
letter and keep the first line under 72 characters. No rigid format —
describe what the change does and why.

## Adding a New Application Wrapper

1. Create `applications/<service>/` directory.
2. Follow the existing pattern (e.g., `applications/redis/`):
   - Define a typed interface matching the service client.
   - Implement `Close(ctx)`, `DSN(db)`, `MustDSN(db)`, `CreateDB(ctx, name)`.
   - Validate all DDL identifiers before passing them to the service.
   - Add `Example*` testable examples.
   - Add versioned integration tests in `applications/<service>/versions/`.
3. Update the application table in `README.md`.
4. Add the well-known image reference to `images/images.go`.

## Testing

- **Unit tests**: pure Go tests — run with `go test ./...` (no Docker needed).
- **Integration tests**: require Docker — run with:

  ```sh
  go test -v ./applications/...
  ```

- **Examples**: run all testable examples:

  ```sh
  go test -run Example ./applications/... .
  ```

- Before opening a PR, ensure all tests pass and lint is clean.

## Pull Request Process

1. Ensure your branch is up to date with `master`.
2. Run `golangci-lint run ./...` — zero issues.
3. Run `go build ./...` — clean compilation.
4. Run tests — pass or be explicitly skipped when Docker is unavailable.
5. Update `README.md` if your change affects the public API or the list of
   supported applications.
6. The PR description should explain **what** and **why**, not **how**.
7. A maintainer will review your PR. Please address feedback promptly.

## AI-Assisted Development

Commits authored or assisted by AI agents (e.g., GitHub Copilot, Claude Code,
ChatGPT, etc.) are welcome under the following conditions:

- **Full human responsibility**: The person who pushes the commit bears full
  responsibility for its correctness, safety, and compliance with project
  conventions — the same as any hand-written commit.
- **Responsibility through the PR lifecycle**: The human author is expected to
  respond to review feedback, fix issues, and drive the pull request to
  approval. AI agents cannot substitute for this.
- **AI opinions are not arguments**: In discussions and code reviews, citing
  what an AI said is not a valid argument. Only the human author's expertise
  and reasoning carry weight.
- **No lowered bar**: Using AI does not reduce the level of expertise or
  understanding required from the author for the change they are making.

## Language

All project communication — code, comments, commit messages, documentation,
issues, and pull requests — **must be in English**. The project is
English-only because maintainers may not speak other languages. Non-English
submissions will not be reviewed.

## Questions?

Open an issue on GitHub with the `question` label, or start a discussion.
