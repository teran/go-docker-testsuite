# go-docker-testsuite — multi-module task runner.
#
# Discovers every Go module in the repository (the root module plus any
# application sub-module that carries its own go.mod) and runs the requested
# command inside each one. Targets:
#
#   make build             go build ./... in every module
#   make test              go test ./... in every module
#   make vet               go vet ./... in every module
#   make lint              golangci-lint run ./... in every module
#   make fmt               gofmt -l check in every module
#   make tidy              go mod tidy in every module
#   make tag v1.5.0        tag the core module as v1.5.0 and each application
#                          module as applications/<name>/v1.5.0
#   make work              (re)write go.work covering every module
#
# GNU Make 3.81 (the default on macOS) is supported.

SHELL := /bin/bash

# Every module dir, root first: "." for the core, then e.g. applications/clickhouse.
MODULES := $(shell find . -name go.mod -not -path './.git/*' -not -path './tools/*' | sed 's|/go.mod||' | sed 's|^\./||' | sort)

# The root module is "."; application modules are everything else.
APP_MODULES := $(filter-out .,$(MODULES))

.PHONY: help build test vet lint fmt tidy tag work

help:
	@echo "go-docker-testsuite multi-module tasks:"
	@echo "  make build    go build ./... in every module"
	@echo "  make test     go test ./... in every module"
	@echo "  make vet      go vet ./... in every module"
	@echo "  make lint     golangci-lint run ./... + replace-directive check"
	@echo "  make fmt      gofmt -l in every module"
	@echo "  make tidy     go mod tidy in every module"
	@echo "  make tag VER  tag core as VER and apps as applications/<name>/VER"
	@echo "  make work     (re)write go.work for all modules"

# Run a shell command inside every module directory.
define run-in-modules
	@for m in $(MODULES); do \
		echo "==> [$$m] $(1)"; \
		(cd "$$m" && $(1)) || exit 1; \
	done
endef

# (Re)write go.work so all modules are visible to tooling during development.
work:
	@go work init 2>/dev/null; \
	for m in $(MODULES); do \
		go work use "$$m"; \
	done
	@echo "go.work updated with: $(MODULES)"

# build/test/vet operate across modules that require the core at a concrete
# (possibly not-yet-published) version, so they first generate go.work to make
# every module resolve the core and its siblings locally.
build: work
	$(call run-in-modules,go build ./...)

test: work
	$(call run-in-modules,go test ./...)

vet: work
	$(call run-in-modules,go vet ./...)

lint: work
	$(call run-in-modules,golangci-lint run ./...)
	@echo "==> checking for replace directives in go.mod"
	@failed=0; \
	for f in $$(find . -name go.mod -not -path './.git/*'); do \
		repl=$$(sed -n 's/^[[:space:]]*replace[[:space:]].*$$/&/p' "$$f"); \
		if [ -n "$$repl" ]; then \
			echo "$$f: replace directives are not allowed (breaks go get for consumers):"; \
			echo "$$repl"; \
			failed=1; \
		fi; \
	done; \
	exit $$failed

fmt:
	@failed=0; \
	for m in $(MODULES); do \
		echo "==> [$$m] gofmt -l"; \
		out=$$(cd "$$m" && gofmt -l .); \
		if [ -n "$$out" ]; then \
			echo "gofmt: unformatted files in $$m:"; \
			echo "$$out"; \
			failed=1; \
		fi; \
	done; \
	exit $$failed

tidy:
	$(call run-in-modules,go mod tidy)

# `make tag <version>` — version is taken from MAKECMDGOALS (e.g. `make tag v1.5.0`).
# The core module is tagged as <version>; each application module is tagged as
# applications/<name>/<version>, matching Go's prefixed-submodule tag scheme.
TAG_ARG := $(filter-out tag,$(MAKECMDGOALS))

# Catch-all so a bare argument like `make tag v1.5.0` doesn't error as an
# unknown target; it is consumed by the tag target above.
%:
	@true

tag:
	@test -n "$(TAG_ARG)" || (echo "usage: make tag <version>  (e.g. make tag v1.5.0)"; exit 1)
	@echo "==> tagging core as $(TAG_ARG)"
	git tag "$(TAG_ARG)"
	@for m in $(APP_MODULES); do \
		echo "==> tagging $$m as $$m/$(TAG_ARG)"; \
		git tag "$$m/$(TAG_ARG)"; \
	done

