package toolspec

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Recorded is a tool call as a recording shows it, in the recording's own names.
type Recorded struct {
	Tool  string
	Input map[string]any
}

// Ungrounded is what a schema holds that the recorded calls do not support: a
// tool or a parameter no recording shows the real harness take (declared, never
// seen), and a call a recording shows that the schema, read in the recording's
// names, would refuse (a recorded call the mock could not play). A tool the
// schema does not declare is left out: recordings hold tools the mock does not
// implement. A harness's test runs it over every recorded run's calls.
func (s Schema) Ungrounded(calls []Recorded) []string {
	seenTool := map[string]bool{}
	seenParam := map[string]bool{}
	var problems []string
	for _, c := range calls {
		for _, t := range s.byRecorded(c.Tool) { // a tool's aliases (Bash and Shell) all stand for the recorded name
			seenTool[t.Name] = true
			in := map[string]any{}
			for k, v := range c.Input {
				name := k
				for _, p := range t.Params {
					if p.recorded() == k {
						name = p.Name
						seenParam[t.Name+"."+p.Name] = true
					}
				}
				in[name] = v
			}
			raw, _ := json.Marshal(in)
			if _, err := (Schema{s.Harness, []Tool{t}}).Check(t.Name, raw); err != nil {
				problems = append(problems, fmt.Sprintf("a recorded call of %s: %v", c.Tool, err))
			}
		}
	}
	for _, t := range s.Tools {
		if !seenTool[t.Name] {
			problems = append(problems, fmt.Sprintf("tool %s is declared and no recording shows it", t.Name))
		}
		for _, p := range t.Params {
			if seenTool[t.Name] && !p.MockOnly && p.Doc == "" && !seenParam[t.Name+"."+p.Name] {
				problems = append(problems, fmt.Sprintf("parameter %s of %s is declared and no recording shows it", p.Name, t.Name))
			}
		}
	}
	sort.Strings(problems)
	return dedupe(problems)
}

func (s Schema) byRecorded(name string) (found []Tool) {
	for _, t := range s.Tools {
		if t.recorded() == name || t.Open && strings.HasSuffix(t.Name, "*") && t.Recorded == "" && strings.HasPrefix(name, strings.TrimSuffix(t.Name, "*")) && (t.Valid == nil || t.Valid(name)) {
			found = append(found, t)
		}
	}
	return found
}

func (t Tool) recorded() string {
	if t.Recorded != "" {
		return t.Recorded
	}
	return t.Name
}

func (p Param) recorded() string {
	if p.Recorded != "" {
		return p.Recorded
	}
	return p.Name
}

func dedupe(s []string) []string {
	var out []string
	for i, x := range s {
		if i == 0 || x != s[i-1] {
			out = append(out, x)
		}
	}
	return out
}
