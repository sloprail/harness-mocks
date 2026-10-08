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

// The forms runs/shell-syntax shows: a double-quoted word is a string, `<` reads
// descriptor 0, `>>` writes 1, `2>&1` copies 2 to the number 1 and is quiet, and a
// command substitution is an argument of its own type whose commands are listed
// after the outer one.
func TestShellFrameArgsModelTheRecordedSyntax(t *testing.T) {
	p := frameArgs(t, `{"command":"echo \"a b\" $(echo inner)"}`)["parsingResult"].(map[string]any)
	cmds := p["executableCommands"].([]map[string]any)
	args := cmds[0]["args"].([]any)
	if args[0].(map[string]any)["type"] != "string" || args[1].(map[string]any)["type"] != "command_substitution" || len(cmds) != 2 || p["hasCommandSubstitution"] != true {
		t.Fatalf("cmds = %v", cmds)
	}
	in := frameArgs(t, `{"command":"cat < in.txt"}`)
	if in["hasInputRedirect"] != true || in["hasOutputRedirect"] != false {
		t.Fatalf("input redirect: %v", in)
	}
	dup := frameArgs(t, `{"command":"ls x 2>&1"}`)
	r := dup["parsingResult"].(map[string]any)["redirects"].([]map[string]any)[0]
	if r["operator"] != ">&" || r["targetNodeType"] != "number" || r["destinationFds"].([]any)[0] != 2 ||
		dup["parsingResult"].(map[string]any)["allRedirectsAreDevNull"] != true || dup["hasOutputRedirect"] != true {
		t.Fatalf("dup: %v", dup)
	}
}

// What no recording shows is refused by name, and what the recordings show is not.
func TestUnmodeledSyntax(t *testing.T) {
	for _, ok := range []string{`echo "a" 'b' c`, `cat < in.txt`, `echo x >> f`, `ls y 2>&1`, `echo $(echo z)`, `a | b; c`, `sh -c 'echo $HOME && ls *'`, `pgrep -f "sleep 2; [t]ouch x"`,
		// runs/shell-compound*: the command line runs in a shell, whatever its shape
		`a && b || c`, `X=7 sh -c 'echo $X'`, `for i in 1 2; do echo "n$i"; done`, `for f in *.txt; do echo "$f"; done`,
		`(echo a; exit 2)`, `{ echo a; echo b >&2; }`, `! false`, `sleep 1 & wait`, `V=hi; echo $V ${V:-d} "$V"`,
		`f() { echo fn; }; f`, "cat > out <<'EOF'\nx\nEOF", "cat <<-EOF\n\tx\n\tEOF", `cat <<< "s"`, `[ -f x ] && echo $((1+2))`,
		`case x in x) echo m;; *) echo n;; esac`, `while [ $i -lt 2 ]; do i=$((i+1)); done`, `echo $1 $@ $?`,
		// runs/shell-glob-tilde: a glob and a ~ are expanded by the shell
		`ls *.txt`, `echo ~`, `echo ~/x [ab].txt ?.txt`} {
		if why := UnmodeledSyntax(ok); why != "" {
			t.Errorf("%q refused: %s", ok, why)
		}
	}
	for _, bad := range []string{"echo `x`", `diff <(a) <(b)`, `((i++))`, `a &> out`, `a >| out`, "{ a; } <<EOF\nx\nEOF"} {
		if UnmodeledSyntax(bad) == "" {
			t.Errorf("%q must be refused", bad)
		}
	}
}
