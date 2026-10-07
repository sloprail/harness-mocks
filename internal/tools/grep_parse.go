package tools

import (
	"errors"
	"fmt"
	"regexp"
	"regexp/syntax"
)

// ParseError is a pattern that does not parse, with the diagnostic of the search tool the way it
// words it ("rg: regex parse error: ...").
type ParseError struct{ Diagnostic string }

func (e *ParseError) Error() string { return e.Diagnostic }

// parseReasons word the ways a pattern fails to parse as ripgrep does.
var parseReasons = map[syntax.ErrorCode]string{
	syntax.ErrMissingParen:          "unclosed group",
	syntax.ErrUnexpectedParen:       "unopened group",
	syntax.ErrMissingBracket:        "unclosed character class",
	syntax.ErrMissingRepeatArgument: "repetition operator missing expression",
	syntax.ErrInvalidRepeatOp:       "repetition operator missing expression",
	syntax.ErrTrailingBackslash:     "incomplete escape sequence, reached end of pattern prematurely",
}

// compileGrep is the pattern's expression, or the *ParseError saying why it does not parse.
func compileGrep(pattern string, ignoreCase bool) (*regexp.Regexp, error) {
	expr := pattern
	if ignoreCase {
		expr = "(?i)" + expr
	}
	re, err := regexp.Compile(expr)
	if err == nil {
		return re, nil
	}
	reason := err.Error()
	var se *syntax.Error
	if errors.As(err, &se) {
		if r, ok := parseReasons[se.Code]; ok {
			reason = r
		}
	}
	return nil, &ParseError{Diagnostic: fmt.Sprintf("rg: regex parse error:\n    (?:%s)\n    ^\nerror: %s", pattern, reason)}
}

// compileGlobs are the expressions of a glob parameter, one per brace alternative.
func compileGlobs(glob string) ([]*regexp.Regexp, error) {
	var out []*regexp.Regexp
	if glob == "" {
		return nil, nil
	}
	for _, p := range expandBraces(glob) {
		g, err := regexp.Compile("^" + globRegexp(p) + "$")
		if err != nil {
			return nil, &ParseError{Diagnostic: "rg: error parsing glob '" + glob + "'"}
		}
		out = append(out, g)
	}
	return out, nil
}
