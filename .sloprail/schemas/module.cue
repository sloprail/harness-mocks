// <dir>/module.yaml: a module's boundary.
#Module: {
	concern!: string & =~"^[^\n]*\\S[^\n]*$" // one line: what the module owns
	home!: [string, ...string] // globs: where everything about the module lives
	api!: [...string]          // packages others may import (may be empty)
}
