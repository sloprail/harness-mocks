package toolexec

import "mvdan.cc/sh/v3/syntax"

// redirect is one redirection as the frame lists it: its operator, the file
// descriptors it writes to or reads, and its target (recorded: a `>` of a word).
// A here-document is a "<<" (also for `<<-`) on descriptor 0 whose target is the name of
// its command, and a here-string has no operator, no descriptor and no target text
// (recorded: runs/shell-compound-forms, runs/shell-compound-more).
func redirect(line string, r *syntax.Redirect, command string) map[string]any {
	switch r.Op {
	case syntax.Hdoc, syntax.DashHdoc:
		return map[string]any{"operator": "<<", "destinationFds": []any{0}, "targetNodeType": "heredoc_redirect", "targetText": command}
	case syntax.WordHdoc:
		return map[string]any{"operator": "", "destinationFds": []any{}, "targetNodeType": "herestring_redirect"}
	}
	fd := 1
	switch r.Op {
	case syntax.RdrIn, syntax.RdrInOut, syntax.DplIn, syntax.Hdoc, syntax.DashHdoc, syntax.WordHdoc:
		fd = 0
	}
	if r.N != nil {
		fd = 0
		for _, c := range r.N.Value {
			fd = fd*10 + int(c-'0')
		}
	}
	target, kind := "", "word"
	if r.Word != nil {
		target, kind = text(line, r.Word), wordType(r.Word) // a number for a dup (2>&1), a word for a file (recorded: runs/shell-syntax)
	}
	return map[string]any{"operator": r.Op.String(), "destinationFds": []any{fd}, "targetNodeType": kind, "targetText": target}
}

// outputRedirects reports whether any redirection writes, and quiet whether every
// one of them leaves no file behind: a copy of one descriptor to another (2>&1) or
// a write to /dev/null (recorded: runs/shell-syntax, where the frame says
// allRedirectsAreDevNull of a 2>&1).
func (p parsed) outputRedirects() (writes, quiet bool) {
	quiet = true
	for _, r := range p.Redirects {
		if fds, _ := r["destinationFds"].([]any); len(fds) > 0 && fds[0] != 0 {
			writes = true
		}
		if r["targetText"] != "/dev/null" && r["operator"] != ">&" && r["operator"] != "<&" && r["targetNodeType"] != "heredoc_redirect" {
			quiet = false
		}
	}
	return writes, quiet
}

// inputRedirects reports whether any redirection reads.
func (p parsed) inputRedirects() bool {
	for _, r := range p.Redirects {
		if fds, _ := r["destinationFds"].([]any); len(fds) > 0 && fds[0] == 0 || r["targetNodeType"] == "herestring_redirect" {
			return true
		}
	}
	return false
}
