package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/sloprail/harness-mocks/claude-mock/internal/runner"
)

// noConversation ends the run the way real Claude Code ends `--resume <id>`
// for a session it has no transcript of (claude 2.1.282): the error's message
// ("No conversation found with session ID: <id>") on stderr, an error result frame on stdout when
// the output format is stream-json, exit status 1.
//
// sr:provides session-resume-unknown/claude
func noConversation(cmd *cobra.Command, noConv *runner.ErrNoConversation) {
	sessionID, msg := noConv.SessionID, noConv.Error()
	fmt.Fprintln(os.Stderr, msg)
	if format, _ := cmd.Flags().GetString(flagOutputFormat); format == "stream-json" {
		frame, _ := json.Marshal(noConversationResult(sessionID, msg))
		fmt.Println(string(frame))
	}
	os.Exit(1)
}
