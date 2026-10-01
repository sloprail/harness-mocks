// <harness>-mock/snapshots/MANIFEST.yaml, written by capture.sh.
#Manifest: {
	version!: string & =~"^[0-9]+(\\.[0-9]+)*$"
	docs?: [string & =~"^https://[^\\s#?]+$"]: {
		version!: string & =~"^[0-9]+(\\.[0-9]+)*$"
		sha256!:  string & =~"^[0-9a-f]{64}$"
	}
}
