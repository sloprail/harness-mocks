// <dir>/module.yaml: a module's boundary.
#Module: {
	home!: [string, ...string] // globs: where everything about the module lives
	api!: [...string]          // packages others may import (may be empty)
}
