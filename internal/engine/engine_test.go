package engine

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoshinoht/shiori/internal/ojson"
	"github.com/hoshinoht/shiori/internal/snapshot"
	"github.com/hoshinoht/shiori/internal/testutil"
)

func mustEngine(t *testing.T, root string) *Engine {
	t.Helper()
	e, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

// TestInterruptedStateHash is the D1/S09 acceptance case: a journal left
// before the primary JSON exists still yields a read-only state hash that
// binds the journal bytes, without writing anything.
func TestInterruptedStateHash(t *testing.T) {
	root := testutil.NewRoot(t, "pending-journal-precreate")
	e := mustEngine(t, root.Path)
	before := testutil.Fingerprint(t, root.Path)
	id := "tx-new"
	doc := func() map[string]any {
		v, err := e.Doctor(DoctorInput{ID: &id})
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err := json.Unmarshal(ojson.Pretty(v), &m); err != nil {
			t.Fatal(err)
		}
		return m
	}
	m := doc()
	if d := testutil.DiffFingerprints(before, testutil.Fingerprint(t, root.Path)); len(d) > 0 {
		t.Fatalf("doctor wrote: %v", d)
	}
	plans := m["plans"].([]any)
	if len(plans) != 1 {
		t.Fatalf("plans = %v", plans)
	}
	entry := plans[0].(map[string]any)
	sh := entry["stateHash"].(string)
	want, err := snapshot.InterruptedStateHash(root.Path, id, snapshot.DefaultLimits,
		[]string{".opencode/workplan/tx-new.json", ".opencode/workplan/tx-new.md"})
	if err != nil || sh != want {
		t.Fatalf("stateHash %s want %s (%v)", sh, want, err)
	}
	if entry["recoveryRequired"] != true {
		t.Fatal("recoveryRequired must be true")
	}
	// Deterministic, and bound to the journal bytes.
	if again := doc()["plans"].([]any)[0].(map[string]any)["stateHash"]; again != sh {
		t.Fatal("state hash not deterministic")
	}
	jp := filepath.Join(root.Path, ".opencode", "workplan", "tx-new.transaction.json")
	data, _ := os.ReadFile(jp)
	os.WriteFile(jp, append(data, ' '), 0o600)
	if changed := doc()["plans"].([]any)[0].(map[string]any)["stateHash"]; changed == sh {
		t.Fatal("state hash must change when the journal changes")
	}
	// read keeps the reference behaviour (not a design change).
	if _, err := e.Read(ReadInput{ID: id}); err == nil || !strings.Contains(err.Error(), "Workplan file not found") {
		t.Fatalf("read: %v", err)
	}
}

// TestReadResponseLimit is D9: above the response frame limit read fails
// with unsupported_capability and a retrieval pointer; the smaller
// projections still work.
func TestReadResponseLimit(t *testing.T) {
	root := testutil.NewRoot(t, "full-valid")
	e := mustEngine(t, root.Path)
	e.MaxResponseBytes = 2048
	_, err := e.Read(ReadInput{ID: "full-plan"})
	if err == nil || !IsUnsupported(err) || !strings.Contains(err.Error(), "includeMarkdown=false") {
		t.Fatalf("want unsupported_capability, got %v", err)
	}
	if _, err := e.Inspect(InspectInput{ID: "full-plan"}); err != nil {
		t.Fatal(err)
	}
}

// TestResumeBudgetSweep is R01: every accepted budget produces a packet
// whose complete text fits, for the stress fixtures and a synthetic
// 13-phase/38-step roadmap, with and without paging. D.1 (contracts §11
// item A) adds: every page progresses, a packet with more than one page
// item never shortens text below the readable minimums or shortens a
// protected path/reference, and a page below the target size only
// carries floor-level item prose (text shrinks before the page).
func TestResumeBudgetSweep(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	budgets := []int{4096, 4097, 5000, 6000, 8191, 8192, 12000, 20000, 64000}
	for i := 0; i < 24; i++ {
		budgets = append(budgets, 4096+rng.Intn(64000-4096+1))
	}
	type target struct {
		e  *Engine
		id string
	}
	var targets []target
	for _, fx := range []struct{ fixture, id string }{{"resume-stress", "stress-plan"}, {"large-paging", "big-plan"}, {"unicode", "unicode-plan"}, {"full-valid", "full-plan"}} {
		root := testutil.NewRoot(t, fx.fixture)
		targets = append(targets, target{mustEngine(t, root.Path), fx.id})
	}
	re, _ := seedRoadmap(t)
	targets = append(targets, target{re, "roadmap"})
	for _, fx := range targets {
		e := fx.e
		for _, b := range budgets {
			for _, limit := range []int{1, 7, 20, 100} {
				b, limit := b, limit
				in := ResumeInput{ID: fx.id, MaxChars: &b, Limit: &limit}
				seen := 0
				for page := 0; page < 400; page++ {
					v, text, err := e.Resume(in)
					if err != nil {
						t.Fatalf("%s budget %d limit %d: %v", fx.id, b, limit, err)
					}
					if n := ojson.UTF16Len(text); n > b {
						t.Fatalf("%s budget %d: %d code units", fx.id, b, n)
					}
					pg, _ := v.Get("page")
					ret, _ := pg.Get("returned")
					r, _ := ret.Float()
					if r > 1 {
						checkReadable(t, fmt.Sprintf("%s budget %d limit %d page %d", fx.id, b, limit, page), v)
					}
					checkTargetOrder(t, fmt.Sprintf("%s budget %d limit %d page %d", fx.id, b, limit, page), v, b, limit)
					seen += int(r)
					next, _ := pg.Get("nextCursor")
					if next.Kind() != ojson.String {
						tot, _ := pg.Get("total")
						tf, _ := tot.Float()
						if seen != int(tf) {
							t.Fatalf("%s budget %d limit %d: paged %d of %d items", fx.id, b, limit, seen, int(tf))
						}
						break
					}
					if r == 0 {
						t.Fatalf("cursor did not progress")
					}
					c := next.Str()
					in.Cursor = &c
				}
			}
		}
	}
}

// TestCursorTamperAndStale covers R03 beyond the corpus: a cursor from a
// changed state is stale; flipping any byte is rejected.
func TestCursorTamperAndStale(t *testing.T) {
	root := testutil.NewRoot(t, "large-paging")
	e := mustEngine(t, root.Path)
	limit := 10
	v, err := e.Inspect(InspectInput{ID: "big-plan", Limit: &limit})
	if err != nil {
		t.Fatal(err)
	}
	pg, _ := v.Get("pagination")
	nc, _ := pg.Get("nextCursor")
	tok := nc.Str()
	for i := 0; i < len(tok); i += 7 {
		b := []byte(tok)
		if b[i] == 'A' {
			b[i] = 'B'
		} else {
			b[i] = 'A'
		}
		bad := string(b)
		if _, err := e.Inspect(InspectInput{ID: "big-plan", Limit: &limit, Cursor: &bad}); err == nil {
			t.Fatalf("tampered cursor accepted at %d", i)
		}
	}
	// Change state: the same cursor is now stale.
	notes := filepath.Join(root.Path, ".opencode", "workplan", "big-plan.md")
	data, _ := os.ReadFile(notes)
	os.WriteFile(notes, append(data, '\n'), 0o600)
	if _, err := e.Inspect(InspectInput{ID: "big-plan", Limit: &limit, Cursor: &tok}); err != errInspectCursorStale {
		t.Fatalf("want stale, got %v", err)
	}
}

// TestConcurrentReads runs every read operation concurrently on one root
// (meaningful under -race) and checks results are identical and nothing
// was written.
func TestConcurrentReads(t *testing.T) {
	root := testutil.NewRoot(t, "large-paging")
	e := mustEngine(t, root.Path)
	before := testutil.Fingerprint(t, root.Path)
	want, _, err := e.Resume(ResumeInput{ID: "big-plan"})
	if err != nil {
		t.Fatal(err)
	}
	wantText := string(ojson.Compact(want))
	errs := make(chan error, 64)
	for i := 0; i < 16; i++ {
		go func() {
			for _, op := range []func() error{
				func() error { _, err := e.Read(ReadInput{ID: "big-plan"}); return err },
				func() error { _, err := e.Inspect(InspectInput{ID: "big-plan"}); return err },
				func() error { _, err := e.Validate(ValidateInput{ID: "big-plan"}); return err },
				func() error { _, err := e.Doctor(DoctorInput{}); return err },
				func() error { _, err := e.List(ListInput{}); return err },
				func() error {
					v, _, err := e.Resume(ResumeInput{ID: "big-plan"})
					if err == nil && string(ojson.Compact(v)) != wantText {
						return errors.New("resume result differs under concurrency")
					}
					return err
				},
			} {
				if err := op(); err != nil {
					errs <- err
					return
				}
			}
			errs <- nil
		}()
	}
	for i := 0; i < 16; i++ {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	if d := testutil.DiffFingerprints(before, testutil.Fingerprint(t, root.Path)); len(d) > 0 {
		t.Fatalf("concurrent reads wrote: %v", d)
	}
}
