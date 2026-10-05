// spec/capabilities/<id>.yaml: a harness behaviour the mocks model.
// Every provider cell is one of:
//   - {docs, runs, deviations?}: supported. It cites that harness's own
//     snapshots: its doc sections by full URL + #anchor, its recorded runs by
//     repo path under <that harness>-mock/, and any deliberate deviation.
//   - {supported: false, reason, docs?, runs?}: the harness lacks it, shown by
//     evidence: recorded runs that attempt the behaviour and show the real
//     harness not doing it (a recording outranks a doc), and/or the doc
//     sections that show the feature absent or cover the area without it.
//     At least one of docs and runs (capability-covered checks that). A bare
//     `false` is not a cell: absence needs evidence.
//   - "pending": not mocked yet. Allowed and tracked, never coverage.
#Capability: {
	statement!: string & =~"\\S"
	providers!: [H=string & =~"^[a-z0-9]+$"]: "pending" | {
		supported?: false
		docs?: [#DocRef, ...#DocRef]
		runs?: [string & =~"^\(H)-mock/snapshots/runs/[a-z][a-z0-9]*(-[a-z0-9]+)*$", ...string & =~"^\(H)-mock/snapshots/runs/[a-z][a-z0-9]*(-[a-z0-9]+)*$"]
		deviations?: [...{
			adr!:       string & =~"^[a-z][a-z0-9]*(-[a-z0-9]+)*$"
			// whose gap it is: `mock-not-modeled` (the mock leaves out what the harness does; the
			// user's call, so adding one needs their words) or `harness-lacks` (the harness itself
			// differs from the statement, shown by a cited doc or recording). Required;
			// a harness-lacks cell cites a doc or a run (capability-grounded/harness-lacks-cited.sh).
			kind!:      "mock-not-modeled" | "harness-lacks"
			statement!: string & =~"\\S"
		}]
		if supported != _|_ {
			// one line: what the harness lacks, as the cited evidence shows it
			reason: string & =~"^[^\\n]*\\S[^\\n]*$"
		}
		if supported == _|_ {
			// supported: both its docs and its runs; deviations are where the
			// mock deliberately differs from what this harness documents or
			// recorded, with the ADR that decides it (judges hold the mock and
			// its tests to the deviation, not to the doc, for exactly that point)
			docs: [#DocRef, ...#DocRef]
			runs: [string & =~"^\(H)-mock/snapshots/runs/[a-z][a-z0-9]*(-[a-z0-9]+)*$", ...string & =~"^\(H)-mock/snapshots/runs/[a-z][a-z0-9]*(-[a-z0-9]+)*$"]
		}
	}
}

#DocRef: string & =~"^https://[^\\s#]+#[a-z0-9-]+$"
