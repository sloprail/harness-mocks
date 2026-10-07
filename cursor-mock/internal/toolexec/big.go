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
// above that (recorded: runs/read-size-cutoff, runs/read-size-cutoff-fine and
// runs/read-size-cutoff-edge: 10000 bytes carried whole, 10001 not, whatever the line
// lengths). The Shell frame's threshold does not apply to Read.
const ReadCarriedBytes = 10000

// contentBlobID is the id a frame gives the content it does not carry: the base64 of
// the content's SHA-256, as the recording's own contents show it.
func contentBlobID(content string) string {
	sum := sha256.Sum256([]byte(content))
	return base64.StdEncoding.EncodeToString(sum[:])
}
