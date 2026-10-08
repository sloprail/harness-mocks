package toolexec

import "testing"

// The forms runs/shell-compound-forms, shell-compound-more and shell-compound-test show: a
// variable is a simple_expansion (braced: expansion), $(( )) an arithmetic_expansion, a
// here-document a "<<" on descriptor 0 whose target is its command's name (and which leaves no
// file behind), a here-string has no operator, and `[` is a test the frame does not list.
func TestShellFrameArgsModelTheRecordedCompoundForms(t *testing.T) {
	p := frameArgs(t, `{"command":"[ -f x ] && echo $V ${V:-d} $((1+2))"}`)["parsingResult"].(map[string]any)
	cmds := p["executableCommands"].([]map[string]any)
	if len(cmds) != 1 {
		t.Fatalf("cmds = %v", cmds)
	}
	var types []any
	for _, a := range cmds[0]["args"].([]any) {
		types = append(types, a.(map[string]any)["type"])
	}
	if len(types) != 3 || types[0] != "simple_expansion" || types[1] != "expansion" || types[2] != "arithmetic_expansion" {
		t.Fatalf("types = %v", types)
	}

	h := frameArgs(t, `{"command":"cat -n <<EOF\nx\nEOF\n"}`)
	r := h["parsingResult"].(map[string]any)["redirects"].([]map[string]any)[0]
	if r["operator"] != "<<" || r["targetNodeType"] != "heredoc_redirect" || r["targetText"] != "cat" || h["hasInputRedirect"] != true ||
		h["parsingResult"].(map[string]any)["allRedirectsAreDevNull"] != true {
		t.Fatalf("heredoc: %v", h)
	}
	file := frameArgs(t, `{"command":"cat > out <<'EOF'\nx\nEOF\n"}`)
	if file["parsingResult"].(map[string]any)["allRedirectsAreDevNull"] != false || file["hasOutputRedirect"] != true {
		t.Fatalf("heredoc to a file: %v", file)
	}
	s := frameArgs(t, `{"command":"cat <<< \"s\""}`)
	r = s["parsingResult"].(map[string]any)["redirects"].([]map[string]any)[0]
	if _, has := r["targetText"]; has || r["operator"] != "" || r["targetNodeType"] != "herestring_redirect" || s["hasInputRedirect"] != true {
		t.Fatalf("here-string: %v", s)
	}
}
