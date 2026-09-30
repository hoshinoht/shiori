package engine

import (
	"testing"

	"github.com/hoshinoht/shiori/internal/testutil"
)

// Stage B open issue 1 (contracts §10): list and path caps between the
// corpus budgets, as measured on the reference (large-paging fixture).
func TestResumeCapsBetweenCorpusBudgets(t *testing.T) {
	root := testutil.NewRoot(t, "large-paging")
	e, _ := New(root.Path)
	cases := []struct{ max, list, paths int }{
		{4096, 4, 16}, {4503, 5, 17}, {5428, 6, 21}, {6316, 7, 24}, {7204, 8, 28}, {8203, 8, 32},
	}
	for _, c := range cases {
		mc := c.max
		v, _, err := e.Resume(ResumeInput{ID: "big-plan", MaxChars: &mc})
		if err != nil {
			t.Fatal(err)
		}
		cp, _ := v.Get("checkpoint")
		bl, _ := cp.Get("blockers")
		tf, _ := v.Get("truncatedFields")
		if len(bl.Elems()) != c.list || len(tf.Elems()) != c.paths {
			t.Errorf("maxChars %d: list %d paths %d, want %d/%d", c.max, len(bl.Elems()), len(tf.Elems()), c.list, c.paths)
		}
	}
}
