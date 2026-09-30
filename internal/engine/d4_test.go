package engine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoshinoht/shiori/internal/ojson"
	"github.com/hoshinoht/shiori/internal/testutil"
)

// Tests for the D.4 approved design changes (contracts §15): the
// compaction advisor (P2) and note rollover (P3). Corpus parity is in
// d4_vectors_test.go.

// d4NoteText builds note i of the synthetic history (invented content). Its
// kind is fixed by i%10 for the 110 notes older than the latest 20:
//
//	0 decision (USER marker)   1 decision ("decided")   2 names open step p2/s8
//	3 [Pinned] marker          4 archive pointer (i==4) 5 names completed p0/s1
//	6 near-miss p2/s8x/xp2/s8  7 quotes the open finding 8 lowercase "user"
//	9 plain
func d4NoteText(i int) string {
	var head string
	switch {
	case i == 4:
		return "Compaction archive: .opencode/workplan/archive/hist/state-0123456789ab-0123456789ab.json"
	case i%10 == 0:
		head = fmt.Sprintf("2026-09-%02d USER DECISION %d: keep the queue", 1+i%28, i)
	case i%10 == 1:
		head = fmt.Sprintf("note %d: the owner decided to ship the parser", i)
	case i%10 == 2:
		head = fmt.Sprintf("note %d: progress on p2/s8, half done", i)
	case i%10 == 3:
		head = fmt.Sprintf("note %d [Pinned]: rollback recipe", i)
	case i%10 == 5:
		head = fmt.Sprintf("note %d: p0/s1 finished", i)
	case i%10 == 6:
		head = fmt.Sprintf("note %d: p2/s8x and xp2/s8 are other names", i)
	case i%10 == 7:
		head = fmt.Sprintf("note %d: see Open finding about the parser cache", i)
	case i%10 == 8:
		head = fmt.Sprintf("note %d: the user asked for a status", i)
	default:
		head = fmt.Sprintf("note %d: routine receipt", i)
	}
	return words(head, 800)
}

const d4Notes = 130

// seedHistory creates a plan with a long, mixed history and a fresh
// checkpoint: an archivable completed phase p0, a completed phase p1 with a
// cancelled step (not archivable), open phase p2, six resolved findings,
// one open finding and 130 notes.
func seedHistory(t *testing.T, notes int) (*Engine, testutil.Root) {
	t.Helper()
	root := testutil.NewRoot(t, "empty-workspace")
	e := mustEngine(t, root.Path)
	step := func(id, status string) string {
		return fmt.Sprintf(`{"id":%q,"title":%q,"target":"src/%s.go","action":%q,"validation":%q,"status":%q}`,
			id, "Step "+id, id, words("Action "+id, 900), words("Validation "+id, 400), status)
	}
	var ns []string
	for i := 0; i < notes; i++ {
		ns = append(ns, fmt.Sprintf("%q", d4NoteText(i)))
	}
	var fs []string
	for i := 0; i < 6; i++ {
		fs = append(fs, fmt.Sprintf(`{"severity":"minor","title":"Resolved finding %d","detail":%q,"status":"resolved"}`, i, words("Detail", 1200)))
	}
	fs = append(fs, `{"severity":"major","title":"Open finding about the parser cache","detail":"still open"}`)
	mustMutate(t, e, "workplan_create", fmt.Sprintf(`{"id":"hist","title":"History","goal":"Keep a long plan small","status":"in_progress",
		"phases":[{"id":"p0","title":"Done","status":"completed","steps":[%s,%s]},
		{"id":"p1","title":"Done with a cancelled step","status":"completed","steps":[%s,%s]},
		{"id":"p2","title":"Open","status":"in_progress","steps":[%s,%s]}],
		"reviewFindings":[%s],"notes":[%s]}`,
		step("s1", "completed"), step("s2", "completed"), step("s3", "completed"), step("s4", "cancelled"),
		step("s8", "draft"), step("s9", "in_progress"), strings.Join(fs, ","), strings.Join(ns, ",")))
	mustMutate(t, e, "workplan_checkpoint", `{"id":"hist","summary":"history seeded","nextAction":"continue p2/s9","phaseId":"p2","stepId":"s9"}`)
	return e, root
}

func d4Doctor(t *testing.T, e *Engine) (ojson.Value, string) {
	t.Helper()
	id := "hist"
	v, err := e.Doctor(DoctorInput{ID: &id})
	if err != nil {
		t.Fatal(err)
	}
	plans, _ := v.Get("plans")
	return plans.Elems()[0], string(ojson.Pretty(v))
}

func d4Resume(t *testing.T, e *Engine, max, limit int) (ojson.Value, string) {
	t.Helper()
	v, text, err := e.Resume(ResumeInput{ID: "hist", MaxChars: &max, Limit: &limit})
	if err != nil {
		t.Fatal(err)
	}
	return v, text
}

func d4File(t *testing.T, root testutil.Root, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root.Path, ".opencode/workplan", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestD4AdvisorReport: doctor has the full advice, resume the compact
// form; the counts restate from the raw plan; advice off, or thresholds
// not crossed, give output byte-identical to the D.4-off engine.
func TestD4AdvisorReport(t *testing.T) {
	e, root := seedHistory(t, d4Notes)
	entry, _ := d4Doctor(t, e)
	a, ok := entry.Get("compactionRecommended")
	if !ok {
		t.Fatalf("no advice: %s", ojson.Compact(entry))
	}
	if err := d4Restate(root.Path, "hist", "workplan_doctor", a); err != nil {
		t.Fatal(err)
	}
	kept := map[string]int{"latest": 20, "decision": 22, "openReference": 22, "pinned": 11, "archivePointer": 1}
	for k, n := range kept {
		if got := numMember(a, "notes", "kept", k); got != n {
			t.Errorf("kept.%s = %d, want %d", k, got, n)
		}
	}
	if got := numMember(a, "notes", "eligible"); got != 110-56 {
		t.Errorf("eligible %d", got)
	}
	if got := strs(getPath(a, "selection", "completedPhaseIds")); strings.Join(got, ",") != "p0" {
		t.Errorf("phases %v", got)
	}
	if got := numMember(a, "terminalSteps", "total"); got != 4 {
		t.Errorf("terminal %d", got)
	}
	if got := numMember(a, "terminalSteps", "archivable"); got != 2 {
		t.Errorf("archivable %d", got)
	}
	if strMember(a, "estimate", "markdown", "treatment") != "generated-refresh" || numMember(a, "estimate", "markdown", "saved") <= 0 {
		t.Errorf("markdown estimate %s", ojson.Compact(getPath(a, "estimate", "markdown")))
	}
	if !containsPrefix(strs(getPath(a, "reasons")), "notes: 54 notes are eligible") {
		t.Errorf("reasons %v", strs(getPath(a, "reasons")))
	}
	// Resume carries the compact form with the same figures.
	r, _ := d4Resume(t, e, 64000, 20)
	c, ok := r.Get("compactionRecommended")
	if !ok {
		t.Fatal("resume has no compact advice")
	}
	if numMember(c, "savedJsonBytes") != numMember(a, "estimate", "json", "saved") || numMember(c, "savedJsonPercent") != numMember(a, "estimate", "json", "percent") ||
		numMember(c, "notes") != 54 || numMember(c, "terminalSteps") != 2 || numMember(c, "resolvedFindings") != 6 {
		t.Errorf("resume advice %s vs doctor %s", ojson.Compact(c), ojson.Compact(a))
	}
	// Off, or a threshold nobody crosses: byte-identical to D.4-off.
	off := *e
	off.noD4 = true
	_, offDoc := d4Doctor(t, &off)
	_, offRes := d4Resume(t, &off, 12000, 20)
	for _, th := range []*CompactionThresholds{{Off: true}, {MinSavingsBytes: 1 << 30}, {Notes: 1000, TerminalPercent: 100, PlanBytes: 1 << 30}} {
		q := *e
		q.Compaction = th
		if _, d := d4Doctor(t, &q); d != offDoc {
			t.Errorf("%+v: doctor differs from D.4-off", *th)
		}
		if _, rs := d4Resume(t, &q, 12000, 20); rs != offRes {
			t.Errorf("%+v: resume differs from D.4-off", *th)
		}
	}
}

func getPath(v ojson.Value, keys ...string) ojson.Value {
	for _, k := range keys {
		v, _ = v.Get(k)
	}
	return v
}

// TestD4AdviceMatchesApply: applying the advised selection writes exactly
// the estimated JSON and Markdown sizes, and the plan is then no longer
// recommended.
func TestD4AdviceMatchesApply(t *testing.T) {
	e, root := seedHistory(t, d4Notes)
	entry, _ := d4Doctor(t, e)
	a := getPath(entry, "compactionRecommended")
	sel := getPath(a, "selection")
	in := fmt.Sprintf(`{"id":"hist","archiveReason":"advice","completedPhaseIds":%s,"noteRollover":%s,"resolvedFindingIndexes":%s}`,
		ojson.Compact(getPath(sel, "completedPhaseIds")), ojson.Compact(getPath(sel, "noteRollover")), ojson.Compact(getPath(sel, "resolvedFindingIndexes")))
	pv := mustMutate(t, e, "workplan_compact", in).Value
	for _, k := range []string{"json", "markdown", "total"} {
		if ojson.Compact(getPath(pv, "estimatedSavings", k)) == nil || numMember(pv, "estimatedSavings", k, "after") != numMember(a, "estimate", k, "after") {
			t.Errorf("%s: preview %s, advice %s", k, ojson.Compact(getPath(pv, "estimatedSavings", k)), ojson.Compact(getPath(a, "estimate", k)))
		}
	}
	h := stateHashFor(t, e, "hist")
	apply := strings.TrimSuffix(in, "}") + fmt.Sprintf(`,"mode":"apply","previewToken":%q,"confirmation":"ARCHIVE_SELECTED_HISTORY","expectedHash":%q}`, strMember(pv, "previewToken"), h)
	out := mustMutate(t, e, "workplan_compact", apply).Value
	if got := len(d4File(t, root, "hist.json")); got != numMember(a, "estimate", "json", "after") || got != numMember(out, "savings", "json", "after") {
		t.Errorf("plan JSON %d bytes, estimated %d, reported %d", got, numMember(a, "estimate", "json", "after"), numMember(out, "savings", "json", "after"))
	}
	if got := len(d4File(t, root, "hist.md")); got != numMember(a, "estimate", "markdown", "after") {
		t.Errorf("Markdown %d bytes, estimated %d", got, numMember(a, "estimate", "markdown", "after"))
	}
	entry, _ = d4Doctor(t, e)
	if has3(entry, "compactionRecommended") {
		t.Errorf("still recommended after applying the advice: %s", ojson.Compact(getPath(entry, "compactionRecommended", "reasons")))
	}
}

// TestD4RolloverPreviewApply covers P3: the selection rule, the token
// binding, the fresh-checkpoint requirement, complete archived originals
// and the resulting notes.
func TestD4RolloverPreviewApply(t *testing.T) {
	e, root := seedHistory(t, d4Notes)
	orig := d4File(t, root, "hist.json")
	preview := func(roll string) ojson.Value {
		return mustMutate(t, e, "workplan_compact", `{"id":"hist","archiveReason":"rollover","noteRollover":`+roll+`}`).Value
	}
	p := preview(`{}`)
	var want []int
	for i := 0; i < d4Notes-20; i++ {
		switch {
		case i == 4, i%10 == 0, i%10 == 1, i%10 == 2, i%10 == 3, i%10 == 7:
		default:
			want = append(want, i)
		}
	}
	if got := string(ojson.Compact(getPath(p, "canonicalSelection", "noteIndexes"))); got != string(ojson.Compact(intsValue(want))) {
		t.Fatalf("selected %s, want %v", got, want)
	}
	if string(ojson.Compact(getPath(p, "canonicalSelection", "noteRollover"))) != `{"keepLatest":20,"pinNoteIndexes":[]}` {
		t.Errorf("canonical rollover %s", ojson.Compact(getPath(p, "canonicalSelection", "noteRollover")))
	}
	if numMember(p, "noteRollover", "selectedCount") != len(want) || numMember(p, "noteRollover", "olderThanLatest") != d4Notes-20 {
		t.Errorf("rollover %s", ojson.Compact(getPath(p, "noteRollover")))
	}
	if numMember(p, "estimatedSavings", "json", "before") != len(orig) {
		t.Errorf("savings before %d, file %d", numMember(p, "estimatedSavings", "json", "before"), len(orig))
	}
	// The token binds keepLatest and the pins.
	p30 := preview(`{"keepLatest":30}`)
	pPin := preview(`{"pinNoteIndexes":[5]}`)
	tok := strMember(p, "previewToken")
	if strMember(p30, "previewToken") == tok || strMember(pPin, "previewToken") == tok {
		t.Fatal("token does not bind the rollover parameters")
	}
	if numMember(pPin, "noteRollover", "kept", "pinned") != 12 || numMember(pPin, "noteRollover", "selectedCount") != len(want)-1 {
		t.Errorf("pin: %s", ojson.Compact(getPath(pPin, "noteRollover")))
	}
	h := stateHashFor(t, e, "hist")
	applyIn := func(roll, token, hash string) string {
		return fmt.Sprintf(`{"id":"hist","archiveReason":"rollover","noteRollover":%s,"mode":"apply","previewToken":%q,"confirmation":"ARCHIVE_SELECTED_HISTORY","expectedHash":%q}`, roll, token, hash)
	}
	auth := &countingAuth{}
	run := func(in string) (Output, error) {
		v, _ := ojson.Parse([]byte(in))
		return runMutation(context.Background(), e, "workplan_compact", v.Value, auth)
	}
	if _, err := run(applyIn(`{"keepLatest":30}`, tok, h)); err == nil || err.Error() != msgWrongToken {
		t.Fatalf("different keepLatest: %v", err)
	}
	// A note appended after the checkpoint: apply needs a fresh one, so
	// no note newer than the checkpoint can be archived.
	mustMutate(t, e, "workplan_update", fmt.Sprintf(`{"id":"hist","expectedHash":%q,"appendNotes":["late note"]}`, h))
	h2 := stateHashFor(t, e, "hist")
	stale := preview(`{}`)
	if _, err := run(applyIn(`{}`, strMember(stale, "previewToken"), h2)); err == nil || err.Error() != msgFreshCheckpoint {
		t.Fatalf("stale checkpoint: %v", err)
	}
	if auth.n != 0 {
		t.Fatalf("refusals reached authorization %d times", auth.n)
	}
	mustMutate(t, e, "workplan_checkpoint", fmt.Sprintf(`{"id":"hist","expectedHash":%q,"summary":"again","nextAction":"continue","phaseId":"p2","stepId":"s9"}`, h2))
	before := d4File(t, root, "hist.json")
	var beforeNotes []string
	{
		s, err := e.load("hist")
		if err != nil {
			t.Fatal(err)
		}
		beforeNotes = s.Plan.Notes
	}
	fresh := preview(`{}`)
	h3 := stateHashFor(t, e, "hist")
	out, err := run(applyIn(`{}`, strMember(fresh, "previewToken"), h3))
	if err != nil {
		t.Fatal(err)
	}
	after := d4File(t, root, "hist.json")
	if numMember(out.Value, "savings", "json", "before") != len(before) || numMember(out.Value, "savings", "json", "after") != len(after) {
		t.Errorf("savings %s, files %d -> %d", ojson.Compact(getPath(out.Value, "savings", "json")), len(before), len(after))
	}
	// The archive holds complete originals.
	arch, err := os.ReadFile(strMember(out.Value, "archivePath"))
	if err != nil {
		t.Fatal(err)
	}
	av := parseT(t, string(arch))
	if strMember(av, "source", "workplanJson") != string(before) {
		t.Error("archive source.workplanJson is not the original plan JSON")
	}
	sel := map[int]bool{}
	for _, r := range getPath(av, "removed", "noteIndexes").Elems() {
		i := numMember(r, "index")
		sel[i] = true
		if strMember(r, "text") != beforeNotes[i] {
			t.Errorf("archived note %d is not the complete original", i)
		}
	}
	if len(sel) != numMember(fresh, "noteRollover", "selectedCount") {
		t.Errorf("archived %d notes, previewed %d", len(sel), numMember(fresh, "noteRollover", "selectedCount"))
	}
	// Remaining notes: the kept ones in order, then the archive pointer.
	s, err := e.load("hist")
	if err != nil {
		t.Fatal(err)
	}
	var keep []string
	for i, n := range beforeNotes {
		if !sel[i] {
			keep = append(keep, n)
		}
	}
	keep = append(keep, "Compaction archive: "+strings.TrimPrefix(strMember(out.Value, "archivePath"), root.Path+"/"))
	if strings.Join(s.Plan.Notes, "\x00") != strings.Join(keep, "\x00") {
		t.Errorf("notes after rollover: %d, want %d", len(s.Plan.Notes), len(keep))
	}
	if s.Plan.Notes[len(s.Plan.Notes)-2] != "late note" {
		t.Error("the note newer than the first checkpoint was not kept among the latest")
	}
}

// TestD4RolloverInput: the new input member on both surfaces.
func TestD4RolloverInput(t *testing.T) {
	cases := []struct{ in, msg string }{
		{`{"noteRollover":{},"noteIndexes":[1]}`, msgRolloverWithIndexes},
		{`{"noteRollover":{"keepLatest":0}}`, "Too small: expected number to be >=1"},
		{`{"noteRollover":{"keepLatest":10001}}`, "Too big: expected number to be <=10000"},
		{`{"noteRollover":{"keepLatest":2.5}}`, "Invalid input: expected int, received number"},
		{`{"noteRollover":{"keepLatest":"20"}}`, "Invalid input: expected number, received string"},
		{`{"noteRollover":{"keep":20}}`, `Unrecognized key: "keep"`},
		{`{"noteRollover":{"pinNoteIndexes":[-1]}}`, "Too small: expected number to be >=0"},
		{`{"noteRollover":true}`, "Invalid input: expected object, received boolean"},
	}
	for _, tool := range []string{"compact", "compact_preview"} {
		for _, s := range []Surface{SurfaceCore, SurfaceNative} {
			for _, c := range cases {
				in := `{"id":"hist","archiveReason":"r",` + strings.TrimPrefix(c.in, "{")
				v, _ := ojson.Parse([]byte(in))
				_, err := ParseMutationInput(tool, v.Value, s)
				var ie *InputError
				if !errors.As(err, &ie) || !strings.Contains(err.Error(), c.msg) {
					t.Errorf("%s %v %s: %v", tool, s, c.in, err)
				}
			}
			v, _ := ojson.Parse([]byte(`{"id":"hist","archiveReason":"r","noteRollover":{"keepLatest":5,"pinNoteIndexes":[1,2]}}`))
			if _, err := ParseMutationInput(tool, v.Value, s); err != nil {
				t.Errorf("%s %v valid input refused: %v", tool, s, err)
			}
		}
	}
	e, _ := seedHistory(t, 30)
	for in, msg := range map[string]string{
		`{"pinNoteIndexes":[30]}`:  "noteRollover.pinNoteIndexes: Note index out of range: 30",
		`{"pinNoteIndexes":[1,1]}`: "noteRollover.pinNoteIndexes contains duplicate indexes",
	} {
		v, _ := ojson.Parse([]byte(`{"id":"hist","archiveReason":"r","noteRollover":` + in + `}`))
		if _, err := runMutation(context.Background(), e, "workplan_compact", v.Value, &countingAuth{}); err == nil || err.Error() != msg {
			t.Errorf("%s: %v", in, err)
		}
	}
	// Nothing older than the latest 20 is eligible here except plain notes:
	// a rollover that selects nothing cannot be applied.
	v, _ := ojson.Parse([]byte(fmt.Sprintf(`{"id":"hist","archiveReason":"r","noteRollover":{"keepLatest":30},"mode":"apply","previewToken":"x","confirmation":"ARCHIVE_SELECTED_HISTORY","expectedHash":%q}`, stateHashFor(t, e, "hist"))))
	if _, err := runMutation(context.Background(), e, "workplan_compact", v.Value, &countingAuth{}); err == nil || !strings.Contains(err.Error(), "Select at least one") {
		t.Errorf("empty rollover apply: %v", err)
	}
}

// TestD4ResumeBudget: every packet fits; the advice is never kept in a
// packet below the D.1 minimums, and a packet without it is byte-identical
// to the D.4-off packet.
func TestD4ResumeBudget(t *testing.T) {
	e, _ := seedHistory(t, d4Notes)
	// A heavy pinned header (the shape of the owner's roadmap) so that the
	// small budgets need every level.
	list := func(prefix string, n, size int) string {
		var out []string
		for i := 0; i < n; i++ {
			out = append(out, fmt.Sprintf("%q", words(fmt.Sprintf("%s %d", prefix, i), size)))
		}
		return "[" + strings.Join(out, ",") + "]"
	}
	h := stateHashFor(t, e, "hist")
	mustMutate(t, e, "workplan_update", fmt.Sprintf(`{"id":"hist","expectedHash":%q,"goal":%q,"scope":%s,"nonGoals":%s,"constraints":%s}`,
		h, words("Goal", 290), list("Scope", 6, 180), list("Non-goal", 5, 150), list("Constraint", 8, 200)))
	mustMutate(t, e, "workplan_checkpoint", fmt.Sprintf(`{"id":"hist","expectedHash":%q,"summary":%q,"nextAction":%q,"phaseId":"p2","stepId":"s9","guardrails":%s,"recentValidation":%s}`,
		stateHashFor(t, e, "hist"), words("Summary", 700), words("Next action", 300), list("Guardrail", 3, 200), list("Validation", 3, 200)))
	off := *e
	off.noD4 = true
	shown, dropped := 0, 0
	for max := 4096; max <= 16000; max += d31Step(max) {
		for _, limit := range []int{1, 8, 20} {
			on, text := d4Resume(t, e, max, limit)
			if ojson.UTF16Len(text) > max {
				t.Fatalf("%d/%d: %d over budget", max, limit, ojson.UTF16Len(text))
			}
			if !has3(on, "compactionRecommended") {
				dropped++
				if _, o := d4Resume(t, &off, max, limit); o != text {
					t.Fatalf("%d/%d: advice dropped but the packet differs from D.4-off", max, limit)
				}
				continue
			}
			shown++
			if belowMinimums(on) {
				t.Fatalf("%d/%d: advice kept in a packet below the D.1 minimums", max, limit)
			}
		}
	}
	t.Logf("advice shown in %d packets, dropped in %d", shown, dropped)
	if shown == 0 || dropped == 0 {
		t.Fatal("the budgets did not exercise both keeping and dropping the advice")
	}
}

// TestD4NoEligibleHistoryIdentical: fixtures and a short plan without
// qualifying history give byte-identical resume and doctor output.
func TestD4NoEligibleHistoryIdentical(t *testing.T) {
	check := func(label string, e *Engine, id string) {
		off := *e
		off.noD4 = true
		for _, max := range []int{4096, 12000, 64000} {
			_, a, err1 := e.Resume(ResumeInput{ID: id, MaxChars: &max})
			_, b, err2 := off.Resume(ResumeInput{ID: id, MaxChars: &max})
			if (err1 == nil) != (err2 == nil) || a != b {
				t.Errorf("%s resume %d differs", label, max)
			}
		}
		d1, _ := e.Doctor(DoctorInput{})
		d2, _ := off.Doctor(DoctorInput{})
		if string(ojson.Pretty(d1)) != string(ojson.Pretty(d2)) {
			t.Errorf("%s doctor differs", label)
		}
	}
	for fixture, id := range map[string]string{"minimal-valid": "minimal", "full-valid": "full-plan", "large-paging": "big-plan", "resume-stress": "stress-plan", "handwritten-md": "hand-plan"} {
		root := testutil.NewRoot(t, fixture)
		check(fixture, mustEngine(t, root.Path), id)
	}
	e, _ := seedHistory(t, 25)
	check("25 notes", e, "hist")
}

func TestParseCompactionThresholds(t *testing.T) {
	th, err := ParseCompactionThresholds("min-savings-kib=8,notes=10,terminal-percent=30,plan-kib=64,keep-notes=5")
	if err != nil {
		t.Fatal(err)
	}
	if *th != (CompactionThresholds{MinSavingsBytes: 8 << 10, Notes: 10, TerminalPercent: 30, PlanBytes: 64 << 10, KeepNotes: 5}) {
		t.Fatalf("%+v", *th)
	}
	if th, _ := ParseCompactionThresholds("off"); !th.Off {
		t.Fatal("off")
	}
	for _, bad := range []string{"", "notes", "notes=0", "notes=x", "terminal-percent=101", "keep-notes=10001", "size=1"} {
		if _, err := ParseCompactionThresholds(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}
