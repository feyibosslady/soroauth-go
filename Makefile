# Makefile for the tasks CONTRIBUTING.md and ARCHITECTURE.md describe.
#
# Every target fails loudly: no recipe swallows an error, and targets that
# check something (fmt, vectors-check) exit non-zero when the check fails
# rather than printing a warning and moving on.
#
# Run `make` or `make help` for the list.

GO   ?= go
NPM  ?= npm
NODE ?= node

# The binary `make build` writes, relative to the repository root.
BIN_DIR := bin
BIN     := $(BIN_DIR)/soroauth

.PHONY: all help fmt vet test build vectors vectors-check e2e clean

# The default target runs exactly what a pull request has to pass before the
# golden-vector drift check, which needs Node and the network.
all: fmt vet test

help:
	@echo "targets:"
	@echo "  make all           fmt, vet and test (the default)"
	@echo "  make fmt           fail if any Go file is not gofmt-clean"
	@echo "  make vet           go vet ./..."
	@echo "  make test          go test ./..."
	@echo "  make build         build the CLI to $(BIN)"
	@echo "  make vectors       regenerate testdata/vectors from the pinned JS SDK"
	@echo "  make vectors-check regenerate and fail if the committed vectors changed"
	@echo "  make e2e           build the test contract and run the live testnet suite"
	@echo "  make clean         remove $(BIN_DIR)/"

# gofmt -l prints the files that need formatting; this target turns that output
# into a failure, which is what CI's gofmt step does.
fmt:
	@unformatted="$$(gofmt -l .)"; \
	if [ -n "$$unformatted" ]; then \
		echo "these files are not gofmt clean:"; \
		echo "$$unformatted"; \
		exit 1; \
	fi

vet:
	$(GO) vet ./...

test:
	$(GO) test ./...

build:
	$(GO) build -o $(BIN) ./cmd/soroauth

# Golden vectors are generated, committed artefacts. Regenerating them is safe;
# `vectors-check` is the one that proves the committed files match, which is
# what CI runs.
vectors:
	cd testdata/gen && $(NPM) ci && $(NODE) gen.mjs

vectors-check: vectors
	@if ! git diff --exit-code -- testdata/vectors; then \
		echo; \
		echo "The committed golden vectors differ from freshly generated ones."; \
		echo "Never edit a vector by hand. Commit the regenerated files with the"; \
		echo "generator change, or investigate why the reference output moved."; \
		exit 1; \
	fi

# The e2e suite needs the contract wasm built first, and stellar-cli to build
# it. Both failures are explicit rather than a confusing test error later.
e2e:
	@command -v stellar >/dev/null 2>&1 || { \
		echo "stellar-cli 28.0.0 is required to build the e2e test contract"; \
		exit 1; \
	}
	cd e2e/contracts && stellar contract build
	$(GO) test -tags e2e -v ./e2e/...

clean:
	rm -rf $(BIN_DIR)
