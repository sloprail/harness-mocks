// spec/capabilities/<id>.yaml: a harness behaviour the mocks model.
// Every provider cell is one of:
//   - {docs, runs, deviations?}: supported. It cites that harness's own
//     snapshots: its doc sections by full URL + #anchor, its recorded runs by
//     repo path under <that harness>-mock/, and any deliberate deviation.
//   - {supported: false, reason, docs}: the harness lacks it, and says so:
//     docs are the sections that show the feature absent, or that cover the
//     area without it. A bare `false` is not a cell: absence needs evidence.
//   - "pending": not mocked yet. Allowed and tracked, never coverage.
#Capability: {
	statement!: string & =~"\\S"
	providers!: [H=string & =~"^[a-z0-9]+$"]: "pending" | #Unsupported | {
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

#Unsupported: {
	supported!: false
	// one line: what the harness lacks, as the cited docs show it
	reason!: string & =~"^[^\\n]*\\S[^\\n]*$"
	docs!:   [#DocRef, ...#DocRef]
}

#DocRef: string & =~"^https://[^\\s#]+#[a-z0-9-]+$"
