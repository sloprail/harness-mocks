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

# The e2e suite commits in throwaway repos; a runner with no global identity needs one. It is
# environment for the test process only: nothing may write the machine's global git config to run
# the tests (copied from CI onto a developer's machine, that rewrote their real ~/.gitconfig and
# every later commit of theirs carried the test identity).
test-e2e: export GIT_AUTHOR_NAME := harness-mocks-test
test-e2e: export GIT_AUTHOR_EMAIL := harness-mocks-test@sloprail.invalid
test-e2e: export GIT_COMMITTER_NAME := harness-mocks-test
test-e2e: export GIT_COMMITTER_EMAIL := harness-mocks-test@sloprail.invalid
# E2E_TEST_FLAGS are more `go test` flags (CI's macOS jobs pass -skip for the replays whose timing the
# runner's timers cannot keep: see .github/workflows/test.yml).
E2E_TEST_FLAGS ?=

test-e2e: build-race
	cd .. && A10N_CLAUDE_MOCK_TEST_BINARY=$(abspath $(CLAUDE_MOCK)) \
		go test -race -count=1 -v $(E2E_TEST_FLAGS) \
		./claude-mock/e2e/...
