package toolexec

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/sloprail/harness-mocks/internal/tools"
)

// lines is how many lines a text has: a final line without a newline counts.
func lines(s string) int {
	n := strings.Count(s, "\n")
	if s != "" && !strings.HasSuffix(s, "\n") {
		n++
	}
	return n
}

// Edit is one change a write made to a file.
type Edit struct {
	OldString string `json:"old_string"`
	NewString string `json:"new_string"`
}

// replaced is the file a StrReplace makes: its old text, which must be there
// once, put by the new.
func replaced(c Call, dir string) (string, error) {
	text, err := tools.ReadFile(c.Path(dir))
	if err != nil {
		return "", err
	}
	if strings.Count(text, c.Replace[0]) != 1 {
		return "", errors.New("cursor-mock: a StrReplace's old text must be in the file exactly once (other cases are not recorded)")
	}
	return strings.Replace(text, c.Replace[0], c.Replace[1], 1), nil
}

// diff is a file's change as the call reports it: the whole old text out and the
// whole new text in as one hunk, as recorded for a file made and a one-line
// edit (runs/file-tools); a change that leaves lines as they were is not
// recorded, so its diff would differ in its context.
func diff(path, old, new string, existed bool) string {
	from := "a/" + path
	if !existed {
		from = "/dev/null"
	}
	count := func(n int) string {
		if n == 1 {
			return ""
		}
		return "," + strconv.Itoa(n)
	}
	ol, nl := splitLines(old), splitLines(new)
	var b strings.Builder
	fmt.Fprintf(&b, "--- %s\n+++ b/%s\n@@ -1%s +1%s @@", from, path, count(len(ol)), count(len(nl)))
	for _, l := range ol {
		b.WriteString("\n-" + l)
	}
	for _, l := range nl {
		b.WriteString("\n+" + l)
	}
	return b.String()
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}

// trimShared drops the longest common prefix, then the longest common suffix
// of what is left, from both texts.
func trimShared(a, b string) (string, string) {
	p := 0
	for p < len(a) && p < len(b) && a[p] == b[p] {
		p++
	}
	a, b = a[p:], b[p:]
	s := 0
	for s < len(a) && s < len(b) && a[len(a)-1-s] == b[len(b)-1-s] {
		s++
	}
	return a[:len(a)-s], b[:len(b)-s]
}
