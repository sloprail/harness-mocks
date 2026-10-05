package replay

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	core "github.com/sloprail/harness-mocks/internal/replay"
)

// conversation is one recorded transcript: the agent's turns, and the prompt
// it was started with.
type conversation struct {
	id     string
	prompt string
	agent  core.Agent
}

// conversations are the transcripts a sample holds besides the main
// session's: Cursor keeps a sub-agent's in a directory of its own named by its
// id, beside the session's (agent-transcripts/<id>/<id>.jsonl), and no file that
// says which call started it.
func conversations(dir, session string) ([]conversation, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil
	}
	var out []conversation
	for _, e := range entries {
		if !e.IsDir() || e.Name() == session {
			continue
		}
		records, err := readJSONL(filepath.Join(dir, e.Name(), e.Name()+".jsonl"))
		if err != nil {
			return nil, err
		}
		agent, err := modelTurns(records)
		if err != nil {
			return nil, &Unbuildable{Reason: "sub-agent: " + err.Error()}
		}
		out = append(out, conversation{id: e.Name(), prompt: firstQuery(records), agent: agent})
	}
	return out, nil
}

// firstQuery is what the first user record asked: the text between the
// <user_query> tags of the record's text, the way Cursor wraps a prompt.
func firstQuery(records []map[string]any) string {
	for _, rec := range records {
		if rec["role"] != "user" {
			continue
		}
		msg, _ := rec["message"].(map[string]any)
		blocks, _ := msg["content"].([]any)
		for _, b := range blocks {
			block, _ := b.(map[string]any)
			text, _ := block["text"].(string)
			_, rest, ok := strings.Cut(text, "<user_query>\n")
			if !ok {
				return ""
			}
			q, _, _ := strings.Cut(rest, "\n</user_query>")
			return q
		}
	}
	return ""
}

// attach is the agent with the sub-agent each of its spawns started, and
// theirs in turn. A spawn started the conversation whose first user record is
// the spawn's prompt: Cursor records no other link between them. A prompt that
// two conversations share is not told apart, so it is an error; a spawn that
// no conversation answers keeps no sub-agent (the call itself was refused).
func attach(a core.Agent, convs *[]conversation) (core.Agent, error) {
	out := core.Agent{Calls: append([]core.Call(nil), a.Calls...), Final: a.Final}
	for i, c := range out.Calls {
		if c.Tool != core.ToolSpawn {
			continue
		}
		prompt, _ := c.Input["message"].(string)
		var found []int
		for j, cv := range *convs {
			if cv.prompt == prompt && prompt != "" {
				found = append(found, j)
			}
		}
		if len(found) > 1 {
			return core.Agent{}, &Unbuildable{Reason: fmt.Sprintf("two sub-agents were started with the prompt %q: the transcripts do not say which call started which", prompt)}
		}
		if len(found) == 0 {
			continue
		}
		cv := (*convs)[found[0]]
		*convs = append((*convs)[:found[0]], (*convs)[found[0]+1:]...)
		sub, err := attach(cv.agent, convs)
		if err != nil {
			return core.Agent{}, err
		}
		out.Calls[i].Sub = &sub
	}
	return out, nil
}
