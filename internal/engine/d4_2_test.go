package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/hoshinoht/shiori/internal/ojson"
	"github.com/hoshinoht/shiori/internal/testutil"
)

// Tests for D.4.2 (contracts §17, approved 2026-10-01). The resume budget
// rule (item 2) is swept by TestD4ResumeBudget and TestD4ComparatorOnCorpus.

func pointerNotes(notes []string) []string {
	var out []string
	for _, n := range notes {
		if strings.HasPrefix(n, archivePointerPrefix) {
			out = append(out, n)
		}
	}
	return out
}

// TestD42ArchivePointerRetention (item 3): successive rollovers keep the
// latest three archive pointer notes; older ones are archived (complete
// text) like ordinary notes, so after an apply the plan holds at most
// four (three kept plus the new one).
func TestD42ArchivePointerRetention(t *testing.T) {
	e, _ := seedHistory(t, 30) // note 4 is an archive pointer
	auth := &countingAuth{}
	run := func(in string) ojson.Value {
		t.Helper()
		v, _ := ojson.Parse([]byte(in))
		out, err := runMutation(context.Background(), e, "workplan_compact", v.Value, auth)
		if err != nil {
			t.Fatal(err)
		}
		return out.Value
	}
	for round := 1; round <= 6; round++ {
		h := stateHashFor(t, e, "hist")
		mustMutate(t, e, "workplan_update", fmt.Sprintf(`{"id":"hist","expectedHash":%q,"appendNotes":["round %d a","round %d b","round %d c"]}`, h, round, round, round))
		mustMutate(t, e, "workplan_checkpoint", fmt.Sprintf(`{"id":"hist","expectedHash":%q,"summary":"round %d","nextAction":"continue","phaseId":"p2","stepId":"s9"}`,
			stateHashFor(t, e, "hist"), round))
		s, err := e.load("hist")
		if err != nil {
			t.Fatal(err)
		}
		before := s.Plan.Notes
		ptrs := pointerNotes(before)
		wantArchived := map[string]bool{}
		if len(ptrs) > KeepArchivePointers {
			for _, n := range ptrs[:len(ptrs)-KeepArchivePointers] {
				wantArchived[n] = true
			}
		}
		roll := `{"keepLatest":2}`
		pv := run(fmt.Sprintf(`{"id":"hist","archiveReason":"round %d","noteRollover":%s}`, round, roll))
		keptPtr := len(ptrs)
		if keptPtr > KeepArchivePointers {
			keptPtr = KeepArchivePointers
		}
		if got := numMember(pv, "noteRollover", "kept", "archivePointer"); got != keptPtr {
			t.Fatalf("round %d: kept.archivePointer %d, want %d", round, got, keptPtr)
		}
		out := run(fmt.Sprintf(`{"id":"hist","archiveReason":"round %d","noteRollover":%s,"mode":"apply","previewToken":%q,"confirmation":"ARCHIVE_SELECTED_HISTORY","expectedHash":%q}`,
			round, roll, strMember(pv, "previewToken"), stateHashFor(t, e, "hist")))
		arch, err := os.ReadFile(strMember(out, "archivePath"))
		if err != nil {
			t.Fatal(err)
		}
		archived := map[string]bool{}
		for _, r := range getPath(parseT(t, string(arch)), "removed", "noteIndexes").Elems() {
			i := numMember(r, "index")
			if strMember(r, "text") != before[i] {
				t.Fatalf("round %d: archived note %d is not the complete original", round, i)
			}
			if strings.HasPrefix(before[i], archivePointerPrefix) {
				archived[before[i]] = true
			}
		}
		if len(archived) != len(wantArchived) {
			t.Fatalf("round %d: archived %d pointer notes, want %d", round, len(archived), len(wantArchived))
		}
		for n := range wantArchived {
			if !archived[n] {
				t.Fatalf("round %d: older pointer %q not archived", round, n)
			}
		}
		s, err = e.load("hist")
		if err != nil {
			t.Fatal(err)
		}
		after := pointerNotes(s.Plan.Notes)
		if want := keptPtr + 1; len(after) != want {
			t.Fatalf("round %d: %d pointer notes after apply, want %d", round, len(after), want)
		}
		if strings.Join(after[:len(after)-1], "\x00") != strings.Join(ptrs[len(ptrs)-keptPtr:], "\x00") {
			t.Fatalf("round %d: the kept pointers are not the latest %d", round, keptPtr)
		}
		t.Logf("round %d: %d pointer notes before, %d archived, %d after", round, len(ptrs), len(archived), len(after))
	}
}

// TestD42AdvisorPointerEstimate (item 3): the advisor counts older pointer
// notes beyond the latest three as eligible, and its figures restate from
// the raw plan.
func TestD42AdvisorPointerEstimate(t *testing.T) {
	e, root := seedHistory(t, d4Notes) // note 4 is an archive pointer
	var add []string
	for i := 0; i < 8; i++ {
		add = append(add, fmt.Sprintf("%q", fmt.Sprintf("Compaction archive: .opencode/workplan/archive/hist/state-%012d-0123456789ab.json", i)))
	}
	for i := 0; i < 25; i++ {
		add = append(add, fmt.Sprintf("%q", words(fmt.Sprintf("later note %d", i), 300)))
	}
	mustMutate(t, e, "workplan_update", fmt.Sprintf(`{"id":"hist","expectedHash":%q,"appendNotes":[%s]}`, stateHashFor(t, e, "hist"), strings.Join(add, ",")))
	mustMutate(t, e, "workplan_checkpoint", fmt.Sprintf(`{"id":"hist","expectedHash":%q,"summary":"pointers","nextAction":"continue","phaseId":"p2","stepId":"s9"}`, stateHashFor(t, e, "hist")))
	entry, _ := d4Doctor(t, e)
	a, ok := entry.Get("compactionRecommended")
	if !ok {
		t.Fatalf("no advice: %s", ojson.Compact(entry))
	}
	if err := d4Restate(root.Path, "hist", "workplan_doctor", a); err != nil {
		t.Fatal(err)
	}
	if got := numMember(a, "notes", "kept", "archivePointer"); got != KeepArchivePointers {
		t.Errorf("kept.archivePointer %d, want %d", got, KeepArchivePointers)
	}
	// 143 notes are older than the latest 20: the 130 seeded notes (65
	// eligible, among them the seeded pointer at index 4, which D.4 kept),
	// the 8 added pointers (the latest 3 kept, 5 eligible) and 5 later notes.
	if got := numMember(a, "notes", "eligible"); got != 65+5+5 {
		t.Errorf("eligible %d, want %d", got, 65+5+5)
	}
	// The advised selection applies to exactly the estimated size.
	sel := getPath(a, "selection")
	in := fmt.Sprintf(`{"id":"hist","archiveReason":"advice","completedPhaseIds":%s,"noteRollover":%s,"resolvedFindingIndexes":%s}`,
		ojson.Compact(getPath(sel, "completedPhaseIds")), ojson.Compact(getPath(sel, "noteRollover")), ojson.Compact(getPath(sel, "resolvedFindingIndexes")))
	pv := mustMutate(t, e, "workplan_compact", in).Value
	apply := strings.TrimSuffix(in, "}") + fmt.Sprintf(`,"mode":"apply","previewToken":%q,"confirmation":"ARCHIVE_SELECTED_HISTORY","expectedHash":%q}`, strMember(pv, "previewToken"), stateHashFor(t, e, "hist"))
	mustMutate(t, e, "workplan_compact", apply)
	if got := len(d4File(t, root, "hist.json")); got != numMember(a, "estimate", "json", "after") {
		t.Errorf("plan JSON %d bytes, estimated %d", got, numMember(a, "estimate", "json", "after"))
	}
}

// TestD42Expectations: D.4.2 changes no oracle vector. The pin file is
// empty and TestCorpusParity (whose D.4 split runs every read vector with
// the advice on and off) requires the D.4 list to stay empty as well.
func TestD42Expectations(t *testing.T) {
	data, err := os.ReadFile(testutil.Testdata("d4_2/expectations.json"))
	if err != nil {
		t.Fatal(err)
	}
	var f d1File
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatal(err)
	}
	if f.Vectors == nil || len(f.Vectors) != 0 || !strings.Contains(f.Note, "§17") {
		t.Fatalf("d4_2 expectations: %d vectors, note %q", len(f.Vectors), f.Note)
	}
	if n := len(d4Expectations(t)); n != 0 {
		t.Fatalf("d4 expectations list %d vectors; D.4.2 expects none", n)
	}
}
