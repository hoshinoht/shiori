package testutil

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// The corpus was generated on APFS, where `UPPER` normalizes to `upper`
// and the lookup opens `UPPER.json` anyway. On a case-sensitive
// filesystem the same call reports the plan as missing (contracts §10,
// "Case-insensitive lookup is inherited from the filesystem").
const (
	caseLookupFixture = "list-mixed"
	caseLookupMissing = "Workplan file not found: $ROOT/.opencode/workplan/upper.json"
)

var (
	caseOnce      sync.Once
	caseSensitive bool
	caseOracleMsg string
)

// CaseSensitive reports whether the fixture filesystem (where NewRoot
// materialises roots) distinguishes names by case.
func CaseSensitive() bool {
	caseOnce.Do(func() {
		base := "/private/tmp"
		if st, err := os.Stat(base); err != nil || !st.IsDir() {
			base = os.TempDir()
		}
		dir, err := os.MkdirTemp(base, "sh-case-")
		if err != nil {
			return
		}
		defer os.RemoveAll(dir)
		if err := os.WriteFile(filepath.Join(dir, "probe"), nil, 0o644); err != nil {
			return
		}
		_, err = os.Stat(filepath.Join(dir, "PROBE"))
		caseSensitive = os.IsNotExist(err)
		var v struct {
			Expect struct {
				Message string `json:"message"`
			} `json:"expect"`
		}
		data, err := os.ReadFile(Testdata("vectors", "hash", "fixtures", "list-mixed--UPPER.json"))
		if err == nil && json.Unmarshal(data, &v) == nil {
			caseOracleMsg = v.Expect.Message
		}
	})
	return caseSensitive
}

// AdaptCaseLookup rewrites oracle text recorded on APFS for a fixture
// into what a case-sensitive filesystem reports: the `UPPER` plan lookup
// becomes "not found". Both the plain message and its JSON-escaped form
// (inside recorded output documents) are rewritten. It reports whether
// anything changed; on a case-insensitive filesystem it never does.
func AdaptCaseLookup(fixture, s string) (string, bool) {
	if fixture != caseLookupFixture || !CaseSensitive() || caseOracleMsg == "" {
		return s, false
	}
	out := strings.ReplaceAll(s, caseOracleMsg, caseLookupMissing)
	out = strings.ReplaceAll(out, jsonInner(caseOracleMsg), jsonInner(caseLookupMissing))
	return out, out != s
}

func jsonInner(s string) string {
	b, _ := json.Marshal(s)
	return string(b[1 : len(b)-1])
}

// CaseLookupMissing reports whether output from a fixture carries the
// case-sensitive "not found" for the `UPPER` plan, so pins of Go output
// recorded on APFS do not apply to it.
func CaseLookupMissing(fixture, s string) bool {
	if fixture != caseLookupFixture || !CaseSensitive() {
		return false
	}
	return strings.Contains(s, "/.opencode/workplan/upper.json")
}
