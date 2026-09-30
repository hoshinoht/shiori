package engine

import (
	"testing"
)

func TestCursorRoundTrip(t *testing.T) {
	pid := "phase-2"
	c := resumeCursor{stateHash: "ef23629bf762ef3fa0f23594a009fc0391f50335875d3ceb26b44d4114884e69", maxChars: 12000, limit: 3, phaseID: &pid, offset: 3}
	tok := encodeCursor(resumeDomain, c.fields())
	got, err := parseResumeCursor(tok)
	if err != nil {
		t.Fatal(err)
	}
	if got.offset != 3 || got.limit != 3 || got.maxChars != 12000 || !eqPtr(got.phaseID, filterDigest(&pid)) || got.stepID != nil {
		t.Fatalf("round trip %+v", got)
	}
	// A cursor is domain-bound: an inspect cursor never parses as resume.
	ic := inspectCursor{stateHash: c.stateHash, limit: 3, offset: 3}
	if _, err := parseResumeCursor(encodeCursor(inspectDomain, ic.fields())); err == nil {
		t.Fatal("cross-domain cursor accepted")
	}
}

// FuzzCursorDecode: arbitrary tokens never panic; any accepted token
// re-encodes to itself (so no two tokens alias one position).
func FuzzCursorDecode(f *testing.F) {
	h := "ef23629bf762ef3fa0f23594a009fc0391f50335875d3ceb26b44d4114884e69"
	f.Add(encodeCursor(inspectDomain, inspectCursor{stateHash: h, limit: 50, offset: 50}.fields()))
	f.Add(encodeCursor(resumeDomain, resumeCursor{stateHash: h, maxChars: 4096, limit: 5, offset: 5}.fields()))
	f.Add("!!not-base64!!")
	f.Add("")
	f.Add("eyJ2ZXJzaW9uIjoxfQ")
	f.Fuzz(func(t *testing.T, tok string) {
		if c, err := parseInspectCursor(tok); err == nil {
			if again := encodeCursor(inspectDomain, c.fields()); again != tok {
				t.Fatalf("inspect token %q re-encodes as %q", tok, again)
			}
		}
		if c, err := parseResumeCursor(tok); err == nil {
			// Filters are stored as digests; re-encode from stored values.
			fields := c.fields()
			fields[4].Value = ptrValue(c.phaseID)
			fields[5].Value = ptrValue(c.stepID)
			if again := encodeCursor(resumeDomain, fields); again != tok {
				t.Fatalf("resume token %q re-encodes as %q", tok, again)
			}
		}
	})
}
