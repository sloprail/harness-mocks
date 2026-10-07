package tools

import (
	"bytes"
	"encoding/json"
	"strings"
)

// SearchLink is one page a search found.
type SearchLink struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

// SearchReminder ends the text of a search result: the agent is told to cite what it found.
const SearchReminder = "REMINDER: You MUST include the sources above in your response to the user using markdown hyperlinks."

// SearchReport is the text a search is answered with: the query, the pages found as a JSON list of
// title and url, the findings drawn from them, and the reminder to cite the sources.
//
// sr:capability web-search-tool
func SearchReport(query string, links []SearchLink, findings string) string {
	if links == nil {
		links = []SearchLink{}
	}
	var list bytes.Buffer
	enc := json.NewEncoder(&list)
	enc.SetEscapeHTML(false) // a title's "&" stays as it is
	_ = enc.Encode(links)
	return "Web search results for query: \"" + query + "\"\n\nLinks: " + strings.TrimSuffix(list.String(), "\n") + "\n\n" + findings + "\n\n\n" + SearchReminder
}
