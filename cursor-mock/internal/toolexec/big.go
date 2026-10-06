package toolexec

import (
	"crypto/sha256"
	"encoding/base64"
)

// BigBytes is how large the output of a command, or the content of a file, may be
// before the harness keeps it in a file of its own and the frame does not carry it:
// the fileOutputThresholdBytes every Shell frame names (recorded:
// runs/compaction-transcript-continuity, a 53900-byte output kept in a file and a 7588-byte
// file read whole; where between them a read stops carrying its content is not recorded,
// and the command's threshold is the one number the harness gives).
const BigBytes = 40000

// contentBlobID is the id a frame gives the content it does not carry: the base64 of
// the content's SHA-256, as the recording's own contents show it.
func contentBlobID(content string) string {
	sum := sha256.Sum256([]byte(content))
	return base64.StdEncoding.EncodeToString(sum[:])
}
