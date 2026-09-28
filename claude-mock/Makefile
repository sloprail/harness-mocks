BIN_DIR      := ../bin
CLAUDE_MOCK  := $(BIN_DIR)/a10n-claude-mock

.PHONY: build build-race test-unit test-e2e

build:
	@mkdir -p $(BIN_DIR)
	go build -o $(CLAUDE_MOCK) .

# The e2e suite drives a race-instrumented build, so a data race in the mock
# (background tasks, concurrent sub-agents) fails CI rather than passing by luck.
build-race:
	@mkdir -p $(BIN_DIR)
	go build -race -o $(CLAUDE_MOCK) .

test-unit:
	cd .. && go test -race -count=1 \
		./claude-mock/internal/...

test-e2e: build-race
	cd .. && A10N_CLAUDE_MOCK_TEST_BINARY=$(abspath $(CLAUDE_MOCK)) \
		go test -race -count=1 -v \
		./claude-mock/e2e/...
