package tools

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSearchReportKeepsTitlesAsTheyAre(t *testing.T) {
	got := SearchReport("q", []SearchLink{{Title: "A & B <c>", URL: "https://x.org/?a=1&b=2"}}, "found it")
	assert.Equal(t, "Web search results for query: \"q\"\n\nLinks: [{\"title\":\"A & B <c>\",\"url\":\"https://x.org/?a=1&b=2\"}]\n\nfound it\n\n\n"+SearchReminder, got)
	assert.Contains(t, SearchReport("q", nil, "none"), "Links: []\n\nnone")
}
