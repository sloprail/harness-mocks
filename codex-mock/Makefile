# How many times go test runs each test (CI passes COUNT=3 to surface timing-flaky tests).
COUNT        ?= 1

BIN_DIR     := ../bin
CODEX_MOCK  := $(BIN_DIR)/a10n-codex-mock

.PHONY: build build-race test-unit test-e2e

build:
	@mkdir -p $(BIN_DIR)
	go build -o $(CODEX_MOCK) .

build-race:
	@mkdir -p $(BIN_DIR)
	go build -race -o $(CODEX_MOCK) .

# The shared core's tests run with it: the mock is an adapter on them.
test-unit:
	cd .. && go test -race -count=$(COUNT) \
		./internal/... \
		./codex-mock/internal/...

# The e2e suite builds its own race-instrumented binary and drives it; the
# hooks it replays are shell scripts that need jq and git.
test-e2e:
	cd .. && go test -race -count=$(COUNT) -v \
		./codex-mock/e2e/...
