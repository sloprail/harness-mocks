# How many times go test runs each test (CI passes COUNT=3 to surface timing-flaky tests).
COUNT        ?= 1

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
	cd .. && go test -race -count=$(COUNT) \
		./claude-mock/internal/...

# The e2e suite commits in throwaway repos; a runner with no global identity needs one. It is
# environment for the test process only: nothing may write the machine's global git config to run
# the tests (copied from CI onto a developer's machine, that rewrote their real ~/.gitconfig and
# every later commit of theirs carried the test identity).
test-e2e: export GIT_AUTHOR_NAME := harness-mocks-test
test-e2e: export GIT_AUTHOR_EMAIL := harness-mocks-test@sloprail.invalid
test-e2e: export GIT_COMMITTER_NAME := harness-mocks-test
test-e2e: export GIT_COMMITTER_EMAIL := harness-mocks-test@sloprail.invalid
test-e2e: build-race
	cd .. && A10N_CLAUDE_MOCK_TEST_BINARY=$(abspath $(CLAUDE_MOCK)) \
		go test -race -count=$(COUNT) -v \
		./claude-mock/e2e/...
