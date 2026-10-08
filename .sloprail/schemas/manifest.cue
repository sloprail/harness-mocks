// <harness>-mock/snapshots/MANIFEST.yaml, written by capture.sh.
//
// Two things that used to share one version are kept apart. The harness binary a recording was
// made with is per run (runs/<name>/run.yaml); `pin` is only which binary capture.sh runs next,
// so moving it invalidates nothing. A doc page is frozen by its own sha256 (and the date it was
// fetched): only a page whose sha256 changed puts the capabilities citing it back in question.
#Manifest: {
	pin!: string & =~"^[0-9]+(\\.[0-9]+)*$"
	docs?: [string & =~"^https://[^\\s#?]+$"]: {
		sha256!:  string & =~"^[0-9a-f]{64}$"
		fetched!: string & =~"^[0-9]{4}-[0-9]{2}-[0-9]{2}$"
	}
}
