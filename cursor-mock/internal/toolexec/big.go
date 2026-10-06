package toolexec

import (
	"crypto/sha256"
	"encoding/base64"
)

// BigBytes is how large the output of a command may be before the harness keeps it
// in a file of its own and the frame does not carry it: the fileOutputThresholdBytes
// every Shell frame names.
const BigBytes = 40000

// A Read carries the content of a file up to ReadCarriedBytes and names it by an id
// from ReadOmittedBytes on: the two sizes recorded (runs/compaction-transcript-continuity:
// a 7602-byte file, in runs/schedule-wakeup-ask, read whole, 53900-byte files not). Where between them a read stops
// carrying its content is not recorded, and the Shell frame's threshold is not shown to
// apply to Read, so the call fails rather than guess.
const (
	ReadCarriedBytes = 7602
	ReadOmittedBytes = 53900
)

// contentBlobID is the id a frame gives the content it does not carry: the base64 of
// the content's SHA-256, as the recording's own contents show it.
func contentBlobID(content string) string {
	sum := sha256.Sum256([]byte(content))
	return base64.StdEncoding.EncodeToString(sum[:])
}
