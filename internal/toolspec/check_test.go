package toolspec

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

var schema = Schema{Harness: "demo", Tools: []Tool{
	{Name: "Shell", Params: []Param{
		{Name: "command", Type: String, Required: true},
		{Name: "block_until_ms", Type: Integer, Values: []any{0}},
		{Name: "description", Type: String},
	}, Answers: map[Kind]string{Missing: "runs/demo-missing"}},
	{Name: "ext__*", Open: true, Valid: func(n string) bool { return len(n) > len("ext__") }},
	{Name: "Read", Params: []Param{{Name: "file_path", Type: String, Required: true}}},
}}

func check(name, input string) ([]Issue, error) { return schema.Check(name, json.RawMessage(input)) }

func refusedWith(t *testing.T, err error, kind Kind, mentions string) {
	t.Helper()
	var e *Error
	if !errors.As(err, &e) || len(e.Issues) == 0 || e.Issues[0].Kind != kind || !strings.Contains(err.Error(), mentions) {
		t.Fatalf("err = %v, want a refusal of kind %q mentioning %q", err, kind, mentions)
	}
}

func TestAValidCallPasses(t *testing.T) {
	if a, err := check("Shell", `{"command":"ls","block_until_ms":0,"description":"x"}`); err != nil || len(a) != 0 {
		t.Fatalf("answered %v, err %v", a, err)
	}
}

func TestAnUnknownToolIsRefused(t *testing.T) {
	_, err := check("Teleport", `{}`)
	refusedWith(t, err, UnknownTool, "Teleport")
}

func TestAnUnknownParameterIsRefused(t *testing.T) {
	_, err := check("Read", `{"file_path":"a","limit":3}`)
	refusedWith(t, err, UnknownParam, "limit")
}

func TestAValueOfTheWrongTypeIsRefused(t *testing.T) {
	_, err := check("Shell", `{"command":5}`)
	refusedWith(t, err, WrongType, "command")
	_, err = check("Shell", `{"command":"ls","block_until_ms":1.5}`)
	refusedWith(t, err, WrongType, "block_until_ms")
	_, err = check("Shell", `["not","an","object"]`)
	refusedWith(t, err, WrongType, "JSON object")
}

func TestAnOptionValueTheMockDoesNotImplementIsRefused(t *testing.T) {
	_, err := check("Shell", `{"command":"ls","block_until_ms":5000}`)
	refusedWith(t, err, BadValue, "block_until_ms")
}

func TestAMissingRequiredParameterIsRefusedUnlessTheHarnessAnswersIt(t *testing.T) {
	_, err := check("Read", `{}`)
	refusedWith(t, err, Missing, "file_path")
	answered, err := check("Shell", `{}`)
	if err != nil || len(answered) != 1 || answered[0].Kind != Missing || answered[0].Param != "command" {
		t.Fatalf("answered %v, err %v", answered, err)
	}
}

// An answered kind does not excuse another mistake in the same call.
func TestAnAnsweredKindDoesNotExcuseAnotherMistake(t *testing.T) {
	_, err := check("Shell", `{"bogus":1}`)
	refusedWith(t, err, UnknownParam, "bogus")
}

func TestUngroundedFindsWhatNoRecordingShows(t *testing.T) {
	calls := []Recorded{{Tool: "Shell", Input: map[string]any{"command": "ls"}}, {Tool: "Other", Input: nil}}
	got := strings.Join(schema.Ungrounded(calls), "\n")
	for _, want := range []string{"tool Read is declared", "parameter description of Shell", "parameter block_until_ms of Shell"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	bad := schema.Ungrounded([]Recorded{{Tool: "Shell", Input: map[string]any{"command": 3}}})
	if !strings.Contains(strings.Join(bad, "\n"), "a recorded call of Shell") {
		t.Errorf("a recorded call the schema refuses is not reported: %v", bad)
	}
}

func TestAPrefixToolTakesAnyNameWithThePrefixAndAnyArguments(t *testing.T) {
	if _, err := check("ext__server__tool", `{"text":"x","n":3}`); err != nil {
		t.Fatal(err)
	}
	_, err := check("ext__server__tool", `[1]`)
	refusedWith(t, err, WrongType, "not a JSON object")
	_, err = check("other__server__tool", `{}`)
	refusedWith(t, err, UnknownTool, "other__server__tool")
}

func TestAPrefixToolRefusesANameItsValidRuleRefuses(t *testing.T) {
	_, err := check("ext__", `{}`)
	refusedWith(t, err, UnknownTool, "ext__")
}
