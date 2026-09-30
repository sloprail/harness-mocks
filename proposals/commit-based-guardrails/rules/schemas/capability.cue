// spec/capabilities/<id>.yaml: a harness behaviour the mocks model.
// Every provider is `false` or cites that harness's own snapshots: its doc
// sections by full URL + #anchor, its recorded runs by repo path under
// <that harness>-mock/.
#Capability: {
	statement!: string & =~"\\S"
	providers!: [H=string & =~"^[a-z0-9]+$"]: false | {
		docs!: [#DocRef, ...#DocRef]
		runs!: [string & =~"^\(H)-mock/snapshots/runs/[a-z][a-z0-9]*(-[a-z0-9]+)*$", ...string & =~"^\(H)-mock/snapshots/runs/[a-z][a-z0-9]*(-[a-z0-9]+)*$"]
	}
}

#DocRef: string & =~"^https://[^\\s#]+#[a-z0-9-]+$"
