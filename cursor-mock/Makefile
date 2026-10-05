BIN_DIR     := ../bin
CURSOR_MOCK := $(BIN_DIR)/a10n-cursor-mock

.PHONY: build build-race test-unit test-e2e

build:
	@mkdir -p $(BIN_DIR)
	go build -o $(CURSOR_MOCK) .

# The e2e suite drives a race-instrumented build, so a data race in the mock
# fails CI rather than passing by luck.
build-race:
	@mkdir -p $(BIN_DIR)
	go build -race -o $(CURSOR_MOCK) .

test-unit:
	cd .. && go test -race -count=1 \
		./cursor-mock/internal/...

test-e2e: build-race
	cd .. && A10N_CURSOR_MOCK_TEST_BINARY=$(abspath $(CURSOR_MOCK)) \
		go test -race -count=1 -v \
		./cursor-mock/e2e/...
