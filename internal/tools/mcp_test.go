package tools

import "testing"

func TestMCPName(t *testing.T) {
	for name, want := range map[string]string{
		"mcp__browser__fill_form":             "browser/fill_form",
		"mcp__plugin_x_browser__browser_fill": "plugin_x_browser/browser_fill",
		"mcp__brave-search__web":              "brave-search/web",
		"mcp__a__b__c":                        "a/b__c",
		"mcp__browser":                        "",
		"mcp____tool":                         "",
		"mcp__server__":                       "",
		"Bash":                                "",
	} {
		server, tool, ok := MCPName(name)
		got := ""
		if ok {
			got = server + "/" + tool
		}
		if got != want {
			t.Errorf("MCPName(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestMCPDisplayName(t *testing.T) {
	for in, want := range map[string]string{"fill_form": "Fill Form", "download_file": "Download File", "web-search": "Web Search", "x": "X"} {
		if got := MCPDisplayName(in); got != want {
			t.Errorf("MCPDisplayName(%q) = %q, want %q", in, got, want)
		}
	}
}
