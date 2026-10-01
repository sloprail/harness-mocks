package tools

import "strings"

// View is the part of a file a read returns.
type View struct {
	// Content is the selected lines, joined by newlines.
	Content string
	// StartLine is the number of the first selected line (1 is the first).
	StartLine int
	// NumLines is how many lines were selected.
	NumLines int
	// TotalLines is how many lines the file has: its text split at every
	// newline, so a final newline ends a last, empty line.
	TotalLines int
}

// Lines is the lines a file's text is read as: split at every newline.
func Lines(content string) []string { return strings.Split(content, "\n") }

// Read selects limit lines of content from line offset (1 is the first); a zero
// offset reads from the start and a zero limit to the end. An offset past the
// last line selects nothing (NumLines is 0).
func Read(content string, offset, limit int) View {
	all := Lines(content)
	start := 1
	if offset > 0 {
		start = offset
	}
	v := View{StartLine: start, TotalLines: len(all)}
	if start > len(all) {
		return v
	}
	end := len(all)
	if limit > 0 && start-1+limit < end {
		end = start - 1 + limit
	}
	v.Content = strings.Join(all[start-1:end], "\n")
	v.NumLines = end - (start - 1)
	return v
}
