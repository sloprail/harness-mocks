package toolexec

import (
	"encoding/json"
	"testing"
)

func frameArgs(t *testing.T, input string) map[string]any {
	t.Helper()
	return FromScript("Shell", json.RawMessage(input)).ShellFrameArgs("id", "conv", "req")
}

// A pipeline is its simple commands in order, with each word typed as the
// recordings type them (runs/*: a flag is a word, a count a number, a quoted
// string a raw_string), and a redirect lists where it goes.
func TestShellFrameArgsParsePipelinesWordsAndRedirects(t *testing.T) {
	a := frameArgs(t, `{"command":"head -c 900000 /dev/urandom | grep -E 'x' > big.txt"}`)
	if got := a["simpleCommands"].([]string); len(got) != 2 || got[0] != "head" || got[1] != "grep" {
		t.Fatalf("simpleCommands = %v", got)
	}
	p := a["parsingResult"].(map[string]any)
	cmds := p["executableCommands"].([]map[string]any)
	if cmds[0]["fullText"] != "head -c 900000 /dev/urandom" {
		t.Fatalf("fullText = %v", cmds[0]["fullText"])
	}
	args := cmds[0]["args"].([]any)
	if args[0].(map[string]any)["type"] != "word" || args[1].(map[string]any)["type"] != "number" {
		t.Fatalf("args = %v", args)
	}
	if g := cmds[1]["args"].([]any)[1].(map[string]any); g["type"] != "raw_string" || g["value"] != "'x'" {
		t.Fatalf("grep arg = %v", g)
	}
	if a["hasOutputRedirect"] != true || a["hasInputRedirect"] != false || p["hasRedirects"] != true || p["allRedirectsAreDevNull"] != false {
		t.Fatalf("redirects: %v %v", a, p)
	}
	r := p["redirects"].([]map[string]any)[0]
	if r["operator"] != ">" || r["targetText"] != "big.txt" || r["destinationFds"].([]any)[0] != 1 {
		t.Fatalf("redirect = %v", r)
	}
}

// A foreground call waits its block_until_ms (30000 by default) and keeps its stdin
// closed; one left in the background has no limit; the model's description is
// carried.
func TestShellFrameArgsLimitsAndFlags(t *testing.T) {
	fg := frameArgs(t, `{"command":"ls","description":"List","block_until_ms":15000}`)
	if fg["timeout"] != 15000 || fg["isBackground"] != false || fg["closeStdin"] != true || fg["hardTimeout"] != 86400000 || fg["description"] != "List" {
		t.Fatalf("foreground: %v", fg)
	}
	if d := frameArgs(t, `{"command":"ls"}`); d["timeout"] != 30000 || d["description"] != nil {
		t.Fatalf("default: %v", d)
	}
	bg := frameArgs(t, `{"command":"sleep 5","block_until_ms":0}`)
	if bg["timeout"] != 0 || bg["isBackground"] != true || bg["closeStdin"] != false || bg["hardTimeout"] != nil || bg["timeoutBehavior"] != "TIMEOUT_BEHAVIOR_UNSPECIFIED" {
		t.Fatalf("background: %v", bg)
	}
}

// A command line that does not parse is reported as a parse that failed.
func TestShellFrameArgsParseFailure(t *testing.T) {
	p := frameArgs(t, `{"command":"echo 'unterminated"}`)["parsingResult"].(map[string]any)
	if p["parsingFailed"] != true {
		t.Fatalf("parsingResult = %v", p)
	}
}
