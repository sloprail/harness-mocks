package transcript

import (
	"bytes"
	"encoding/json"
)

// Marshal encodes a record without HTML escaping, the way a harness writes its
// transcript: a reader looking for the real text (a notification's markup, a
// hook's "a && b") must find it.
func Marshal(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// Envelope is the bookkeeping a harness writes on every record: field name to
// value (session, working directory, branch, timestamp, version, how the run
// was launched, ...). What the fields are and are called is the harness's.
type Envelope map[string]any

// Stamp is line with every envelope field the record does not carry already,
// and no other change. A line that is not a JSON object is returned as it is.
func (e Envelope) Stamp(line []byte) []byte {
	var rec map[string]any
	if err := json.Unmarshal(line, &rec); err != nil || rec == nil {
		return line
	}
	changed := false
	for k, v := range e {
		if _, ok := rec[k]; !ok {
			rec[k] = v
			changed = true
		}
	}
	if !changed {
		return line
	}
	if b, err := Marshal(rec); err == nil {
		return b
	}
	return line
}

// MarkPrompt sets marks on a prompt record when the run is non-interactive, so
// a reader can tell a prompt handed to a scripted run from a typed one. A
// record keeps a mark it carries already.
func MarkPrompt(rec map[string]any, nonInteractive bool, marks map[string]any) {
	if !nonInteractive {
		return
	}
	for k, v := range marks {
		if _, ok := rec[k]; !ok {
			rec[k] = v
		}
	}
}

// Chain is line with a uuid (minted by newUUID when absent) and a parent link:
// fields uuidKey and parentKey are filled only where the record lacks them, the
// parent with parent, or an explicit null for the first record of a file. It
// returns the record's uuid so the caller can advance the chain. A line that is
// not a JSON object is returned unchanged with an empty uuid.
func Chain(line []byte, parent, uuidKey, parentKey string, newUUID func() string) (out []byte, uuid string) {
	var rec map[string]any
	if err := json.Unmarshal(line, &rec); err != nil || rec == nil {
		return line, ""
	}
	changed := false
	if s, ok := rec[uuidKey].(string); ok && s != "" {
		uuid = s
	} else if uuid = newUUID(); uuid != "" {
		rec[uuidKey] = uuid
		changed = true
	}
	if _, present := rec[parentKey]; !present {
		if parent != "" {
			rec[parentKey] = parent
		} else {
			rec[parentKey] = nil
		}
		changed = true
	}
	if !changed {
		return line, uuid
	}
	if b, err := Marshal(rec); err == nil {
		return b, uuid
	}
	return line, uuid
}
