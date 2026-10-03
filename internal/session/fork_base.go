package session

import "bytes"

// HistoryBase is where a fork's history stays when the fork's own file does
// not copy it: the source session and how much of its transcript the fork
// continues from. A harness whose forks carry the history by reference (not
// as copied records, as Fork does) writes this into the fork's header.
type HistoryBase struct {
	// From is the source session's id.
	From string
	// Records is how many records of the source the fork continues from, Bytes
	// how long that part of the source is.
	Records, Bytes int
}

// ForkBase is the history base of a fork of session from taken now: the whole
// of its transcript src, as it stands.
func ForkBase(from string, src []byte) HistoryBase {
	return HistoryBase{From: from, Records: bytes.Count(src, []byte("\n")), Bytes: len(src)}
}

// Through is the part of the source transcript src the base names: the
// history the fork continues, which the source itself is not changed by.
func (b HistoryBase) Through(src []byte) []byte {
	if b.Bytes > len(src) {
		return src
	}
	return src[:b.Bytes]
}
