package tools

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFetchRefused(t *testing.T) {
	for url, refused := range map[string]bool{
		"http://localhost:1/": true, "https://LocalHost/x": true, "https://intranet/wiki": true,
		"https://example.com": false, "http://docs.example.org/a?b=c": false, "https://127.0.0.1/": false,
	} {
		assert.Equal(t, refused, FetchRefused(url), url)
	}
}

func TestFetchableURL(t *testing.T) {
	assert.True(t, FetchableURL("https://example.com/x"))
	assert.False(t, FetchableURL("ftp://example.com/x"))
	assert.False(t, FetchableURL("example.com"))
	assert.False(t, FetchableURL("https:///x"))
}
