package evidence

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Tree is the git tree of a project root's working state (uncommitted and
// untracked, non-ignored files included), without the workplan directory.
type Tree struct {
	OID   string
	Scope map[string]*string // scope path -> entries digest (nil: absent)
}

// TreeTimeout bounds one tree computation.
var TreeTimeout = 10 * time.Second

// GitBinary is the git executable (tests may override it).
var GitBinary = "git"

// ErrNoGit: no git work tree; records get no tree and state "unknown".
var ErrNoGit = errors.New("not inside a git work tree")

// Snapshot computes root's working tree without writing to the repository:
// git works on a copied index with a private, empty object directory (no
// alternates, so not even object mtimes are freshened). exclude is removed
// from the tree; each scope path gets a digest of its index entries.
func Snapshot(ctx context.Context, root, exclude string, scope []string) (*Tree, error) {
	ctx, cancel := context.WithTimeout(ctx, TreeTimeout)
	defer cancel()
	g := &gitRun{ctx: ctx, dir: root}
	out, err := g.run(nil, "rev-parse", "--is-inside-work-tree", "--show-prefix", "--path-format=absolute", "--git-path", "index")
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNoGit, err)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 3 || lines[0] != "true" {
		return nil, ErrNoGit
	}
	prefix, index := lines[1], lines[2]

	tmp, err := os.MkdirTemp("", "shiori-evidence-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	if err := os.Mkdir(filepath.Join(tmp, "objects"), 0o700); err != nil {
		return nil, err
	}
	tmpIndex := filepath.Join(tmp, "index")
	if err := copyFile(index, tmpIndex); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	g.env = []string{"GIT_INDEX_FILE=" + tmpIndex, "GIT_OBJECT_DIRECTORY=" + filepath.Join(tmp, "objects")}
	if _, err := g.run(nil, "add", "-A", "--", "."); err != nil {
		return nil, err
	}
	if exclude != "" {
		if _, err := g.run(nil, "rm", "-r", "--cached", "-q", "--ignore-unmatch", "--", exclude); err != nil {
			return nil, err
		}
	}
	args := []string{"write-tree", "--missing-ok"}
	if prefix != "" {
		args = append(args, "--prefix="+prefix)
	}
	oid, err := g.run(nil, args...)
	if err != nil {
		// An empty project subtree: hash the empty tree.
		if prefix == "" {
			return nil, err
		}
		if oid, err = g.run(strings.NewReader(""), "hash-object", "-t", "tree", "--stdin"); err != nil {
			return nil, err
		}
	}
	t := &Tree{OID: strings.TrimSpace(oid), Scope: map[string]*string{}}
	for _, p := range scope {
		listing, err := g.run(nil, "ls-files", "-s", "-z", "--", p)
		if err != nil {
			return nil, err
		}
		if listing == "" {
			t.Scope[p] = nil
			continue
		}
		sum := sha256.Sum256([]byte(listing))
		d := hex.EncodeToString(sum[:])
		t.Scope[p] = &d
	}
	return t, nil
}

type gitRun struct {
	ctx context.Context
	dir string
	env []string
}

// run executes git without inherited GIT_* variables, prompts or optional locks.
func (g *gitRun) run(stdin io.Reader, args ...string) (string, error) {
	cmd := exec.CommandContext(g.ctx, GitBinary, append([]string{"--no-optional-locks", "--literal-pathspecs", "-c", "core.fsmonitor=false"}, args...)...)
	cmd.Dir = g.dir
	env := []string{}
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "GIT_") || strings.HasPrefix(kv, "LC_ALL=") {
			continue
		}
		env = append(env, kv)
	}
	cmd.Env = append(append(env, "GIT_TERMINAL_PROMPT=0", "LC_ALL=C"), g.env...)
	cmd.Stdin = stdin
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		if g.ctx.Err() != nil {
			return "", fmt.Errorf("git %s timed out", args[0])
		}
		msg := strings.TrimSpace(errb.String())
		if i := strings.IndexByte(msg, '\n'); i >= 0 {
			msg = msg[:i]
		}
		if msg == "" {
			msg = err.Error()
		}
		return out.String(), fmt.Errorf("git %s: %s", args[0], msg)
	}
	return out.String(), nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
