package childenv

import (
	"reflect"
	"testing"

	"github.com/sloprail/harness-mocks/internal/procexec"
)

// sr:proves subprocess-session-env/cursor
func TestAShellCommandSeesThisSessionAndAnInheritedHarnessMarkPassesThrough(t *testing.T) {
	inherited := []string{"PATH=/bin", "CURSOR_CONVERSATION_ID=outer", "CURSOR_INVOKED_AS=outer", "CURSOR_AGENT=outer"}
	got := procexec.Env(inherited, Identity("sid-1"), Defaults())
	want := []string{"PATH=/bin", "CURSOR_AGENT=outer", "CURSOR_CONVERSATION_ID=sid-1", "CURSOR_INVOKED_AS=cursor-agent"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("env = %v, want %v", got, want)
	}
	if got := procexec.Env([]string{"PATH=/bin"}, Identity("sid-2"), Defaults()); !reflect.DeepEqual(got,
		[]string{"PATH=/bin", "CURSOR_AGENT=1", "CURSOR_CONVERSATION_ID=sid-2", "CURSOR_INVOKED_AS=cursor-agent"}) {
		t.Fatalf("env = %v", got)
	}
}
