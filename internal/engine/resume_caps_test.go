package engine

import (
	"testing"

	"github.com/hoshinoht/shiori/internal/testutil"
)

// Stage B open issue 1 (contracts §10): list and path caps between the
// corpus budgets, as measured on the reference (large-paging fixture).
// D.1 (contracts §11 item A) keeps both caps; it only changes which
// degradation level is chosen, so the large-paging packets at these
// budgets no longer truncate anything. The pinned-list cap is still
// observable there; the listed-path cap is checked on resume-stress,
// which still truncates.
func TestResumeCapsBetweenCorpusBudgets(t *testing.T) {
	root := testutil.NewRoot(t, "large-paging")
	e, _ := New(root.Path)
	stress := testutil.NewRoot(t, "resume-stress")
	es, _ := New(stress.Path)
	cases := []struct{ max, list, paths int }{
		{4096, 4, 16}, {4503, 5, 17}, {5428, 6, 21}, {6316, 7, 24}, {7204, 8, 28}, {8203, 8, 32},
	}
	for _, c := range cases {
		if resumeListCap(c.max) != c.list || resumePathCap(c.max) != c.paths {
			t.Errorf("maxChars %d: caps %d/%d want %d/%d", c.max, resumeListCap(c.max), resumePathCap(c.max), c.list, c.paths)
		}
		mc := c.max
		v, _, err := e.Resume(ResumeInput{ID: "big-plan", MaxChars: &mc})
		if err != nil {
			t.Fatal(err)
		}
		cp, _ := v.Get("checkpoint")
		bl, _ := cp.Get("blockers")
		if len(bl.Elems()) != c.list {
			t.Errorf("maxChars %d: list %d want %d", c.max, len(bl.Elems()), c.list)
		}
		sv, _, err := es.Resume(ResumeInput{ID: "stress-plan", MaxChars: &mc})
		if err != nil {
			t.Fatal(err)
		}
		tf, _ := sv.Get("truncatedFields")
		cnt, _ := sv.Get("truncatedFieldCount")
		n, _ := cnt.Float()
		want := c.paths
		if int(n) < want {
			want = int(n)
		}
		if n == 0 || len(tf.Elems()) != want {
			t.Errorf("maxChars %d: stress listed paths %d want %d (count %d)", c.max, len(tf.Elems()), want, int(n))
		}
	}
}
