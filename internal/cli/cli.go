// Package cli implements the standalone `shiori` command. It runs the same
// engine in-process (no child, no protocol) under local operator
// authority. Stage B exposes read-only operations only; nothing here can
// write an artifact.
package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/hoshinoht/shiori/internal/engine"
	"github.com/hoshinoht/shiori/internal/model"
	"github.com/hoshinoht/shiori/internal/ojson"
	"github.com/hoshinoht/shiori/internal/snapshot"
)

// Version is the CLI version (set with -ldflags at build time).
var Version = "0.0.0-stage-b"

const usage = `shiori — read-only workplan core (stage B)

Usage:
  shiori <command> [flags] [id]

Commands:
  list                       List plans and classified sidecars
  read <id>                  Read a plan (normalized JSON, selection, Markdown)
  inspect <id>               Page stable phase/step ids and Markdown markers
  validate <id>              Validate structure, links, dependencies, recovery state
  resume <id>                Bounded continuation packet (UTF-16 budget)
  doctor [id]                Read-only diagnostics: roots, sidecars, locks, journals
  version                    Print the version

Common flags:
  --root DIR                 Project root (default: current directory)
  --json                     Machine output: the tool result object only, on stdout
  --input JSON               Raw tool input object (core surface; overrides flags)

Command flags:
  read:     --phase ID --step ID --no-markdown
  inspect:  --phase ID --limit N --cursor TOKEN
  resume:   --max-chars N --limit N --cursor TOKEN --phase ID --step ID
  doctor:   --limit N

Exit status: 0 success; 1 operation error or (validate) an invalid plan;
2 usage error. Reads never prompt and never write.
`

// Run executes the CLI and returns the process exit status.
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "-h", "--help", "help":
		fmt.Fprint(stdout, usage)
		return 0
	case "version", "--version":
		fmt.Fprintln(stdout, "shiori", Version)
		return 0
	case "list", "read", "inspect", "validate", "resume", "doctor":
	default:
		fmt.Fprintf(stderr, "shiori: unknown command %q\n\n%s", cmd, usage)
		return 2
	}

	fs := flag.NewFlagSet("shiori "+cmd, flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", "", "project root")
	jsonOut := fs.Bool("json", false, "machine-readable output")
	rawInput := fs.String("input", "", "raw tool input JSON")
	phase := fs.String("phase", "", "phase id")
	step := fs.String("step", "", "step id")
	noMD := fs.Bool("no-markdown", false, "omit linked Markdown")
	limit := fs.Int("limit", 0, "page size")
	cursor := fs.String("cursor", "", "cursor from the previous page")
	maxChars := fs.Int("max-chars", 0, "resume budget in UTF-16 code units")
	var positional []string
	for {
		if err := fs.Parse(rest); err != nil {
			return 2
		}
		if fs.NArg() == 0 {
			break
		}
		positional = append(positional, fs.Arg(0))
		rest = fs.Args()[1:]
	}
	set := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })

	// Build the core-surface tool input object.
	var input ojson.Value
	if *rawInput != "" {
		p, err := ojson.Parse([]byte(*rawInput))
		if err != nil {
			fmt.Fprintf(stderr, "shiori: --input is not valid JSON: %v\n", err)
			return 2
		}
		input = p.Value
	} else {
		b := ojson.NewObject(8)
		if len(positional) > 1 {
			fmt.Fprintf(stderr, "shiori %s: expected at most one id, got %d arguments\n", cmd, len(positional))
			return 2
		}
		if len(positional) == 1 {
			if cmd == "list" {
				fmt.Fprintln(stderr, "shiori list: takes no id")
				return 2
			}
			b.Set("id", ojson.StringValue(positional[0]))
		}
		str := func(flagName, key string, v string) {
			if set[flagName] {
				b.Set(key, ojson.StringValue(v))
			}
		}
		num := func(flagName, key string, v int) {
			if set[flagName] {
				b.Set(key, ojson.IntValue(int64(v)))
			}
		}
		switch cmd {
		case "read":
			str("phase", "phaseId", *phase)
			str("step", "stepId", *step)
			if *noMD {
				b.Set("includeMarkdown", ojson.BoolValue(false))
			}
		case "inspect":
			str("phase", "phaseId", *phase)
			num("limit", "limit", *limit)
			str("cursor", "cursor", *cursor)
		case "resume":
			num("max-chars", "maxChars", *maxChars)
			num("limit", "limit", *limit)
			str("cursor", "cursor", *cursor)
			str("phase", "phaseId", *phase)
			str("step", "stepId", *step)
		case "doctor":
			num("limit", "limit", *limit)
		}
		for name := range set {
			if !flagAllowed(cmd, name) {
				fmt.Fprintf(stderr, "shiori %s: flag --%s does not apply\n", cmd, name)
				return 2
			}
		}
		input = b.Value()
	}

	// The root is trusted operator context (flag, or workspaceRoot in the
	// core-surface input), never model input on the native surface.
	rootDir := *root
	if wr, ok := input.Get("workspaceRoot"); ok && wr.Kind() == ojson.String && rootDir == "" {
		rootDir = wr.Str()
	}
	if rootDir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return fail(stdout, stderr, *jsonOut, err)
		}
		rootDir = wd
	}
	e, err := engine.New(rootDir)
	if err != nil {
		return fail(stdout, stderr, *jsonOut, err)
	}

	res, text, err := dispatch(e, cmd, input)
	if err != nil {
		return fail(stdout, stderr, *jsonOut, err)
	}
	if *jsonOut {
		if text == "" {
			text = string(ojson.Pretty(res))
		}
		fmt.Fprintln(stdout, text)
	} else {
		human(stdout, cmd, res)
	}
	if cmd == "validate" {
		if v, ok := res.Get("valid"); ok && !v.Bool() {
			return 1
		}
	}
	return 0
}

func flagAllowed(cmd, name string) bool {
	switch name {
	case "root", "json", "input":
		return true
	}
	allowed := map[string][]string{
		"read":    {"phase", "step", "no-markdown"},
		"inspect": {"phase", "limit", "cursor"},
		"resume":  {"max-chars", "limit", "cursor", "phase", "step"},
		"doctor":  {"limit"},
	}
	for _, a := range allowed[cmd] {
		if a == name {
			return true
		}
	}
	return false
}

// dispatch validates input on the core surface and runs the operation.
// text is set when the operation defines its own serialization (resume).
func dispatch(e *engine.Engine, cmd string, input ojson.Value) (ojson.Value, string, error) {
	s := engine.SurfaceCore
	switch cmd {
	case "list":
		in, err := engine.ParseListInput(input, s)
		if err != nil {
			return ojson.Value{}, "", err
		}
		v, err := e.List(in)
		return v, "", err
	case "read":
		in, err := engine.ParseReadInput(input, s)
		if err != nil {
			return ojson.Value{}, "", err
		}
		v, err := e.Read(in)
		return v, "", err
	case "inspect":
		in, err := engine.ParseInspectInput(input, s)
		if err != nil {
			return ojson.Value{}, "", err
		}
		v, err := e.Inspect(in)
		return v, "", err
	case "validate":
		in, err := engine.ParseValidateInput(input, s)
		if err != nil {
			return ojson.Value{}, "", err
		}
		v, err := e.Validate(in)
		return v, "", err
	case "resume":
		in, err := engine.ParseResumeInput(input, s)
		if err != nil {
			return ojson.Value{}, "", err
		}
		return e.Resume(in)
	default:
		in, err := engine.ParseDoctorInput(input, s)
		if err != nil {
			return ojson.Value{}, "", err
		}
		v, err := e.Doctor(in)
		return v, "", err
	}
}

// ErrorClass maps an error to a protocol error class
// (protocol-envelope-v1 errorClass).
func ErrorClass(err error) string {
	var ie *engine.InputError
	var de *model.DecodeError
	var nf *snapshot.NotFoundError
	var ij *snapshot.InvalidJSONError
	msg := err.Error()
	switch {
	case errors.As(err, &ie):
		return "invalid_input"
	case errors.Is(err, snapshot.ErrUnsupported):
		return "unsupported_capability"
	case errors.As(err, &nf):
		return "missing_artifact"
	case errors.As(err, &de), errors.As(err, &ij), strings.Contains(msg, "Migrate ids before"):
		return "invalid_structure"
	case strings.HasPrefix(msg, "Stale or option-mismatched"):
		return "stale_state"
	case strings.HasPrefix(msg, "Invalid workplan") && strings.HasSuffix(msg, "restart without a cursor"),
		strings.HasPrefix(msg, "Phase not found"), strings.HasPrefix(msg, "Step filter"),
		errors.Is(err, model.ErrUnnormalizableID),
		strings.HasPrefix(msg, "Plan file must"), strings.HasPrefix(msg, "Spec file must"):
		return "invalid_input"
	}
	return "internal"
}

func fail(stdout, stderr io.Writer, jsonOut bool, err error) int {
	if !jsonOut {
		fmt.Fprintln(stderr, "shiori:", err)
		return 1
	}
	b := ojson.NewObject(3).
		Set("class", ojson.StringValue(ErrorClass(err))).
		Set("message", ojson.StringValue(err.Error()))
	var issues []model.Issue
	var ie *engine.InputError
	var de *model.DecodeError
	switch {
	case errors.As(err, &ie):
		issues = ie.Issues
	case errors.As(err, &de):
		issues = de.Issues
	}
	if len(issues) > 0 {
		out := make([]ojson.Value, len(issues))
		for i, is := range issues {
			p := is.PathString()
			if p == "" {
				p = "$"
			}
			out[i] = ojson.NewObject(2).Set("path", ojson.StringValue(p)).Set("message", ojson.StringValue(is.Message)).Value()
		}
		b.Set("issues", ojson.ArrayValue(out))
	}
	v := ojson.NewObject(2).Set("ok", ojson.BoolValue(false)).Set("error", b.Value()).Value()
	fmt.Fprintln(stdout, string(ojson.Pretty(v)))
	return 1
}

// itoa is a small helper for human output.
func itoa(v ojson.Value) string {
	if f, ok := v.Float(); ok {
		return strconv.FormatInt(int64(f), 10)
	}
	return v.Str()
}
