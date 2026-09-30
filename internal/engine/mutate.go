package engine

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hoshinoht/shiori/internal/model"
	"github.com/hoshinoht/shiori/internal/ojson"
	"github.com/hoshinoht/shiori/internal/snapshot"
	"github.com/hoshinoht/shiori/internal/storage"
)

// Mutation flow (spec 01 §6, spec 02 §3): Prepare* validates input and
// state, reads the complete artifact set and returns a Prepared intent
// without creating any file, lock or directory. Execute asks the
// Authorizer about exactly that intent and only then commits it through
// the storage engine, which rechecks everything under the locks.

// AuthRequest is what an Authorizer decides on.
type AuthRequest struct {
	Tool      string
	Intent    *storage.Intent
	Digest    string
	Resources storage.Resources
}

// Authorizer grants or refuses a prepared intent. It must not change any
// file. The CLI implements it with a TTY prompt or --yes; the native
// OpenCode host authorizer is stage D.
type Authorizer interface {
	Authorize(ctx context.Context, req AuthRequest) error
}

// ErrDenied is a refused authorization.
var ErrDenied = errors.New("Workplan mutation was not authorized; nothing was changed")

// AllowAll authorizes everything (tests and trusted in-process callers).
type AllowAll struct{}

func (AllowAll) Authorize(context.Context, AuthRequest) error { return nil }

// Prepared is a single-use prepared mutation.
type Prepared struct {
	Tool   string
	Intent *storage.Intent // nil: nothing would change; no authorization needed
	digest string
	used   bool
	// result renders the tool output for the committed state.
	result  func(directorySync bool) (Output, error)
	recheck func() error
}

// Output is a tool result: a JSON object, or text plus metadata (patch).
type Output struct {
	Value    ojson.Value
	Text     string
	Metadata ojson.Value
}

// String is the exact tool output text.
func (o Output) String() string {
	if o.Text != "" {
		return o.Text
	}
	return string(ojson.Pretty(o.Value))
}

// ExecOptions are trusted, non-model collaborators of Execute.
type ExecOptions struct {
	Hooks storage.Hooks
}

// Execute authorizes and commits a prepared mutation.
func (e *Engine) Execute(ctx context.Context, p *Prepared, auth Authorizer, opts ExecOptions) (Output, error) {
	if p.used {
		return Output{}, errors.New("prepared mutation already used; prepare again")
	}
	p.used = true
	if p.Intent == nil {
		return p.result(true)
	}
	if p.Intent.Digest() != p.digest {
		return Output{}, errors.New("prepared mutation changed after preparation; prepare again")
	}
	if err := ctx.Err(); err != nil {
		return Output{}, &storage.CancelledError{Stage: "authorization"}
	}
	if auth == nil {
		return Output{}, ErrDenied
	}
	if err := auth.Authorize(ctx, AuthRequest{Tool: p.Tool, Intent: p.Intent, Digest: p.digest, Resources: p.Intent.Resources()}); err != nil {
		return Output{}, err
	}
	// A late approval after cancellation cannot reactivate the request.
	if err := ctx.Err(); err != nil {
		return Output{}, &storage.CancelledError{Stage: "locking"}
	}
	h := opts.Hooks
	userRecheck := h.Recheck
	h.Recheck = func() error {
		if p.recheck != nil {
			if err := p.recheck(); err != nil {
				return err
			}
		}
		if userRecheck != nil {
			return userRecheck()
		}
		return nil
	}
	res, err := storage.Commit(ctx, p.Intent, h)
	if err != nil {
		return Output{}, err
	}
	return p.result(res.DirectorySync)
}

func finalize(p *Prepared) *Prepared {
	if p.Intent != nil {
		p.digest = p.Intent.Digest()
	}
	return p
}

// Clock is the engine's time source (tests freeze it).
var Clock = func() time.Time { return time.Now() }

func nowISO() string { return Clock().UTC().Format("2006-01-02T15:04:05.000Z") }

// errPendingJournal is the reference refusal for a plan with a journal.
func (e *Engine) errPendingJournal(id string) error {
	return fmt.Errorf("Workplan transaction pending requires explicit recovery at %s", e.absRel(journalRel(id)))
}

// StaleHashError is the reference stale-state refusal.
type StaleHashError struct{ Current string }

func (e *StaleHashError) Error() string {
	return "Stale expectedHash; current stateHash is " + e.Current + ". Reread the plan and recompute the mutation."
}

// DuplicateMembersError refuses to mutate a plan whose stored JSON repeats
// member names (D4): the plan stays readable, writes fail closed.
type DuplicateMembersError struct {
	ID    string
	Paths []string
}

func (e *DuplicateMembersError) Error() string {
	return "Workplan " + e.ID + " has duplicate JSON member names: " + strings.Join(e.Paths, ", ") + ". Remove the duplicates before mutating the plan."
}

// loadForMutation reads the complete artifact set of an existing plan and
// applies the refusals common to every existing-state writer.
func (e *Engine) loadForMutation(raw string, expected *string) (*snapshot.Snapshot, error) {
	id, err := normalizeRequested(raw)
	if err != nil {
		return nil, err
	}
	s, err := e.load(id)
	if err != nil {
		return nil, err
	}
	if s.Journal.Exists {
		return nil, e.errPendingJournal(id)
	}
	if len(s.Plan.Duplicates) > 0 {
		paths := make([]string, len(s.Plan.Duplicates))
		for i, d := range s.Plan.Duplicates {
			if d.Path == "" {
				paths[i] = d.Key
			} else {
				paths[i] = d.Path + "." + d.Key
			}
		}
		return nil, &DuplicateMembersError{ID: id, Paths: paths}
	}
	if expected != nil && *expected != s.StateHash {
		return nil, &StaleHashError{Current: s.StateHash}
	}
	return s, nil
}

// readsOf records the exact state manifest as preconditions.
func readsOf(entries []snapshot.Entry) []storage.ReadEntry {
	out := make([]storage.ReadEntry, 0, len(entries))
	for _, en := range entries {
		out = append(out, storage.ReadEntry{Rel: en.Path, SHA256: en.SHA256, Missing: en.Missing})
	}
	return out
}

// fileMode returns the existing permission bits of a file. A new plan
// JSON or linked Markdown takes the mode of the existing primary plans in
// the workplan root, with owner read/write added, or 0644 minus the
// process umask when there is none (D.3, contracts §13 item 5). Every
// other new file (sidecars, archives) stays 0600.
func (e *Engine) fileMode(rel, kind string) fs.FileMode {
	if st, err := os.Stat(e.absRel(rel)); err == nil {
		return st.Mode().Perm()
	}
	if kind != "plan" && kind != "markdown" {
		return 0o600
	}
	return e.newArtifactMode()
}

// newArtifactMode is the mode for a new plan JSON or linked Markdown.
func (e *Engine) newArtifactMode() fs.FileMode {
	if l, err := e.scanDir(); err == nil {
		for _, name := range l.primary {
			st, err := os.Lstat(filepath.Join(e.dir(), name+".json"))
			if err == nil && st.Mode().IsRegular() {
				return st.Mode().Perm() | 0o600
			}
		}
	}
	return fs.FileMode(0o644 &^ processUmask)
}

// targetSpec is one prospective artifact change.
type targetSpec struct {
	rel        string
	kind       string
	before     []byte
	beforeOK   bool
	after      []byte
	afterOK    bool
	forceWrite bool // keep even when bytes are unchanged
}

// buildIntent assembles the intent; targets whose bytes do not change are
// dropped (no needless replacement).
func (e *Engine) buildIntent(op, id, tx string, specs []targetSpec, reads []storage.ReadEntry) *storage.Intent {
	in := &storage.Intent{
		Operation:     op,
		WorkplanID:    id,
		Root:          e.Root,
		TransactionID: tx,
		CreatedAt:     nowISO(),
		Reads:         reads,
		JournalRel:    journalRel(id),
		JournalStage:  storage.JournalStagePath(snapshot.WorkplanDir, id, tx),
	}
	var rels []string
	for _, t := range specs {
		if !t.forceWrite && t.beforeOK == t.afterOK && string(t.before) == string(t.after) {
			continue
		}
		tg := storage.Target{Rel: t.rel, Kind: t.kind, Before: t.before, BeforeExists: t.beforeOK, After: t.after, AfterExists: t.afterOK, Mode: e.fileMode(t.rel, t.kind)}
		if t.afterOK {
			tg.Stage = storage.StagePath(t.rel, tx, len(in.Targets))
		}
		in.Targets = append(in.Targets, tg)
		rels = append(rels, t.rel)
	}
	if len(in.Targets) == 0 {
		return nil
	}
	in.Dirs = storage.ParentDirs(append(rels, snapshot.WorkplanDir+"/x")...)
	in.Locks = []storage.LockRef{
		storage.NewLockRef("workspace", snapshot.WorkplanDir+"/.workspace-mutation.lock", tx),
		storage.NewLockRef("plan", snapshot.WorkplanDir+"/."+id+".lock", tx),
	}
	return in
}

// overlay maps each target to its after-image (nil = absent) plus the
// journal as absent: the committed state.
func overlay(in *storage.Intent, id string) map[string][]byte {
	m := map[string][]byte{journalRel(id): nil}
	if in == nil {
		return m
	}
	for _, t := range in.Targets {
		if t.AfterExists {
			m[t.Rel] = t.After
		} else {
			m[t.Rel] = nil
		}
	}
	return m
}

// postSnapshot is the snapshot the committed intent produces.
func (e *Engine) postSnapshot(in *storage.Intent, id string) (*snapshot.Snapshot, error) {
	return snapshot.LoadOverlay(e.Root, id, e.Limits, overlay(in, id))
}

func dirSyncValue(ok bool) ojson.Value {
	if ok {
		return ojson.StringValue("supported")
	}
	return ojson.StringValue("unsupported")
}

// claimCheck enforces workspace linkage for new destinations: pending
// journals of other plans claim their targets (unparseable journals fail
// closed), and another plan's linked Markdown owns its path even while
// absent. It runs during preparation and again under the workspace lock.
func (e *Engine) claimCheck(id string, newRels []string, markdownRels []string) error {
	if len(newRels) == 0 {
		return nil
	}
	l, err := e.scanDir()
	if err != nil {
		return err
	}
	// Paths compare case-folded: on case-insensitive filesystems (APFS
	// default) differently cased links alias one file, so ambiguous
	// claims fail closed everywhere (contracts §10, case policy).
	want := map[string]bool{}
	for _, r := range newRels {
		want[strings.ToLower(r)] = true
	}
	r := &snapshot.Reader{Root: e.Root, Limits: e.Limits}
	for _, sc := range l.sidecars {
		if sc.kind != kindTransaction {
			continue
		}
		owner := strings.TrimSuffix(sc.name, ".transaction.json")
		if owner == id {
			continue
		}
		rel := snapshot.WorkplanDir + "/" + sc.name
		a, err := r.ReadFile(rel)
		if err != nil || !a.Exists {
			return fmt.Errorf("Ambiguous pending workplan destination claim at %s", e.absRel(rel))
		}
		parsed, perr := ojson.Parse(a.Bytes)
		if perr != nil {
			return fmt.Errorf("Ambiguous pending workplan destination claim at %s", e.absRel(rel))
		}
		j, ok := model.DecodeJournal(parsed.Value)
		if !ok {
			return fmt.Errorf("Ambiguous pending workplan destination claim at %s", e.absRel(rel))
		}
		for _, t := range j.Targets {
			if want[strings.ToLower(t.Path)] {
				return fmt.Errorf("Workplan destination is claimed by pending transaction %s for %s", j.TransactionID, j.WorkplanID)
			}
		}
	}
	if len(markdownRels) == 0 {
		return nil
	}
	md := map[string]bool{}
	for _, m := range markdownRels {
		md[strings.ToLower(m)] = true
	}
	for _, name := range l.primary {
		other, err := model.NormalizeID(name)
		if err != nil || other == id || other != name {
			continue
		}
		_, p, err := r.LoadPlanDocument(other)
		if err != nil {
			continue
		}
		pf, err := snapshot.NormalizePlanFile(e.Root, p.PlanFile)
		if err != nil {
			continue
		}
		if md[strings.ToLower(pf)] {
			return fmt.Errorf("Plan file destination is already owned by %s: %s", other, e.absRel(pf))
		}
	}
	return nil
}

// newTargets lists the rels of targets absent before (new destinations).
func newTargets(in *storage.Intent) (all, markdown []string) {
	if in == nil {
		return nil, nil
	}
	for _, t := range in.Targets {
		if !t.BeforeExists && t.AfterExists {
			all = append(all, t.Rel)
			if t.Kind == "markdown" {
				markdown = append(markdown, t.Rel)
			}
		}
	}
	return all, markdown
}

// relPath is the project-relative form of an absolute path under root.
func (e *Engine) relPath(p string) string {
	r, err := filepath.Rel(e.Root, p)
	if err != nil {
		return p
	}
	return filepath.ToSlash(r)
}
