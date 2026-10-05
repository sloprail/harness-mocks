package main

import (
	"github.com/spf13/cobra"
)

// flags are cursor-agent's command line, and the mock's own --script. Flags
// that only matter to a real model or an interactive session are accepted and
// ignored, so a command line written for cursor-agent runs unchanged.
type flags struct {
	print, force, yolo, trust, streamPartial, plan, cont, approveMCPs     bool
	outputFormat, model, workspace, script, apiKey, sandbox, mode, resume string
	pluginDirs, addDirs                                                   []string
}

func newRoot() *cobra.Command {
	var f flags
	root := &cobra.Command{
		Use:   "a10n-cursor-mock [prompt...]",
		Short: "Deterministic Cursor agent mock for e2e testing",
		Long: `a10n-cursor-mock is a drop-in replacement for the 'cursor-agent' CLI used in
end-to-end tests. Instead of calling a model it runs a user-supplied scenario
script (--script, or A10N_MOCK_SCRIPT) that prints Cursor stream-json, fires the
hooks in .cursor/hooks.json, runs the tool calls the script asks for and passes
the stream through:

  cursor-agent -p --force --output-format stream-json <prompt>
  ->
  a10n-cursor-mock -p --force --output-format stream-json --script scenario.sh <prompt>`,
		Args:          cobra.ArbitraryArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          func(cmd *cobra.Command, args []string) error { return run(cmd, f, args) },
	}
	p := root.Flags()
	p.BoolVarP(&f.print, "print", "p", false, "print responses to the console (non-interactive; required)")
	p.StringVar(&f.outputFormat, "output-format", "text", "output format with --print: only stream-json is modeled")
	p.BoolVar(&f.streamPartial, "stream-partial-output", false, "accepted, ignored")
	p.StringVar(&f.mode, "mode", "", "accepted, ignored")
	p.BoolVar(&f.plan, "plan", false, "accepted, ignored")
	p.StringVar(&f.resume, "resume", "", "resume the session with this id")
	p.BoolVar(&f.cont, "continue", false, "not modeled")
	p.StringVar(&f.model, "model", "", "accepted, ignored")
	p.BoolVarP(&f.force, "force", "f", false, "accepted, ignored")
	p.BoolVar(&f.yolo, "yolo", false, "accepted, ignored")
	p.BoolVar(&f.trust, "trust", false, "accepted, ignored")
	p.StringVar(&f.workspace, "workspace", "", "workspace directory (default: the current directory)")
	// recorded (runs/add-dir-access, no-add-dir-access, multiroot-workspace): a
	// second root added with --add-dir changes neither what a headless run may
	// read nor the hooks' workspace_roots, so it is accepted and ignored
	p.StringArrayVar(&f.addDirs, "add-dir", nil, "accepted, ignored, only with -p --force (the recorded mode): hooks still name the one workspace root")
	p.BoolVar(&f.approveMCPs, "approve-mcps", false, "approve the project's MCP servers without asking (MCP tool calls are modeled only with it)")
	p.StringVar(&f.apiKey, "api-key", "", "accepted, ignored")
	p.StringVar(&f.sandbox, "sandbox", "", "accepted, ignored")
	p.StringVar(&f.script, "script", "", "the scenario script (default: $A10N_MOCK_SCRIPT)")
	p.StringArrayVar(&f.pluginDirs, "plugin-dir", nil, "load a local plugin directory: its hooks join the project's (repeatable)")
	root.AddCommand(newReplay())
	return root
}
