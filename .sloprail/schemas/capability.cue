// spec/capabilities/<id>.yaml: a harness behaviour the mocks model.
// Every provider is `false` or cites that harness's own snapshots: its doc
// sections by full URL + #anchor, its recorded runs by repo path under
// <that harness>-mock/, and any deliberate deviation from them.
#Capability: {
	statement!: string & =~"\\S"
	providers!: [H=string & =~"^[a-z0-9]+$"]: false | {
		docs!: [#DocRef, ...#DocRef]
		runs!: [string & =~"^\(H)-mock/snapshots/runs/[a-z][a-z0-9]*(-[a-z0-9]+)*$", ...string & =~"^\(H)-mock/snapshots/runs/[a-z][a-z0-9]*(-[a-z0-9]+)*$"]
		// Where the mock deliberately differs from what this harness documents
		// or recorded, and the ADR that decides it. Judges hold the mock and
		// its tests to the deviation, not to the doc, for exactly this point.
		deviations?: [...{
			adr!:       string & =~"^[a-z][a-z0-9]*(-[a-z0-9]+)*$"
			statement!: string & =~"\\S"
		}]
	}
}

#DocRef: string & =~"^https://[^\\s#]+#[a-z0-9-]+$"
