package runner

// Invocation is what a run's own flags name beyond its session.
type Invocation struct {
	// Name is --name: the name the session is given, which a later --resume finds it by.
	Name string
	// Tools is --tools: the only tools the run has (when RestrictTools; none if Tools is empty). A call
	// to another is refused by the mock.
	Tools         []string
	RestrictTools bool
	// PermissionHost is --permission-prompt-tool stdio: the run has someone to ask for permission, so
	// the tools that need a user (AskUserQuestion) are offered to it.
	PermissionHost bool
}
