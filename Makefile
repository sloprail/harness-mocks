BIN_DIR      := ../../bin
CLAUDE_MOCK  := $(BIN_DIR)/a10n-claude-mock

.PHONY: build test-unit test-e2e

build:
	@mkdir -p $(BIN_DIR)
	go build -o $(CLAUDE_MOCK) .

test-unit:
	cd ../.. && go test -race -count=1 \
		./services/claude-mock/internal/...

test-e2e: build
	cd ../.. && A10N_CLAUDE_MOCK_TEST_BINARY=$(abspath $(CLAUDE_MOCK)) \
		go test -race -count=1 -v \
		./services/claude-mock/e2e/...
