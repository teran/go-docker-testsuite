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

.PHONY: help build test vet lint fmt tidy tag-core tag-app work

help:
	@echo "go-docker-testsuite multi-module tasks:"
	@echo "  make build       go build ./... in every module"
	@echo "  make test        go test ./... in every module"
	@echo "  make vet         go vet ./... in every module"
	@echo "  make lint        golangci-lint run ./... + replace-directive check"
	@echo "  make fmt         gofmt -l in every module"
	@echo "  make tidy        go mod tidy in every module"
	@echo "  make tag-core V  tag only the core as V (e.g. v1.6.0)"
	@echo "  make tag-app N V tag applications/N as applications/N/V"
	@echo "  make work        (re)write go.work for all modules"

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

# Tagging for the multi-module layout. The core and each application live on
# independent release cycles, so their tags are placed separately (a core tag
# must exist before an application that requires it can be tagged):
#
#   make tag-core v1.6.0              tag only the core as v1.6.0
#   make tag-app redis v1.3.0         tag applications/redis as applications/redis/v1.3.0
#
# Each target consumes its arguments from MAKECMDGOALS.

# Catch-all so bare arguments like `make tag-core v1.6.0` don't error as
# unknown targets; they are consumed by the tag targets below.
%:
	@true

tag-core:
	@test -n "$(filter-out tag-core,$(MAKECMDGOALS))" || (echo "usage: make tag-core <version>  (e.g. make tag-core v1.6.0)"; exit 1)
	@echo "==> tagging core as $(filter-out tag-core,$(MAKECMDGOALS))"
	git tag "$(filter-out tag-core,$(MAKECMDGOALS))"

tag-app:
	@test "$$(echo '$(MAKECMDGOALS)' | wc -w)" -ge 3 || (echo "usage: make tag-app <name> <version>  (e.g. make tag-app redis v1.3.0)"; exit 1)
	@set -- $(filter-out tag-app,$(MAKECMDGOALS)); \
	app=$$1; ver=$$2; \
	if [ ! -d "applications/$$app" ]; then \
		echo "no such application: $$app"; \
		exit 1; \
	fi; \
	echo "==> tagging applications/$$app as applications/$$app/$$ver"; \
	git tag "applications/$$app/$$ver"

