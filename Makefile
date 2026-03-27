.PHONY: clean generate test

GOCACHE ?= /tmp/gocache
TOOLS_BIN := $(CURDIR)/.bin
MOCKGEN := $(TOOLS_BIN)/mockgen
MOCKGEN_VERSION := v0.6.0

generate:
	@test -x "$(MOCKGEN)" || { \
		mkdir -p "$(TOOLS_BIN)" && \
		GOBIN="$(TOOLS_BIN)" GOCACHE="$(GOCACHE)" go install go.uber.org/mock/mockgen@$(MOCKGEN_VERSION); \
	}
	@mkdir -p mocks
	PATH="$(TOOLS_BIN):$$PATH" GOCACHE="$(GOCACHE)" go generate ./...

test: generate
	GOCACHE="$(GOCACHE)" go test ./...

clean:
	rm -rf .bin
