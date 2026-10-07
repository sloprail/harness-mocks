package hooks

import (
	"encoding/json"

	corehooks "github.com/sloprail/harness-mocks/internal/hooks"
)

// interpretTurn reads the output of a hook around an interactive turn. A beforeSubmitPrompt hook
// refuses the prompt with {"continue": false} (and says why in user_message): recorded, the prompt
// is not submitted, no turn follows and no transcript is written (runs/tui-prompt-blocked). A stop
// hook gives {"followup_message": "..."} to have the agent take it as its next message (recorded:
// runs/tui-stop-followup). Output that is not such a JSON object decides nothing.
//
// sr:docs https://cursor.com/docs/hooks#beforesubmitprompt
// sr:docs https://cursor.com/docs/hooks#stop
func interpretTurn(e Event, out string) Decision {
	if !corehooks.IsJSONOutput(out, isOutputField) {
		return Decision{}
	}
	var p struct {
		Continue    *bool  `json:"continue"`
		UserMessage string `json:"user_message"`
		Followup    string `json:"followup_message"`
	}
	if json.Unmarshal([]byte(out), &p) != nil {
		return Decision{}
	}
	if e == BeforeSubmitPrompt {
		return Decision{Refused: p.Continue != nil && !*p.Continue, Message: p.UserMessage}
	}
	return Decision{Followup: p.Followup}
}
