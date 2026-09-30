// adr/<name>/ADR.md frontmatter. There is no `status`: an ADR in the tree is
// in force. A new setting key is a schema change, so settings stay deliberate.
#ADR: {
	concern!: string & =~"^[^\n]*\\S[^\n]*$" // one line: the index concern-undeclared uses
	sloprails!: [#Rule, ...#Rule]
	modules?: [...string]    // dirs holding a module.yaml
	exceptions?: [...string] // legacy paths (or globs, where the ADR says so); only shrink
	space?: [...string]      // globs of code that must be mapped to modules
	limits?: [string]: int & >0
}

#Rule: string & =~"^(file-guard|gate|context)/[a-z][a-z0-9]*(-[a-z0-9]+)*$"
