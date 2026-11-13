// Origin: nodelogicau/intentions-cli internal/cli/root.go @ b72a395454aa
// (Execute, run, printError, emit and the error envelope; the verb tree and
// the error mapping are episodes')

// Package cli wires the core to a cobra command tree. It owns the
// agent-facing contract: no prompts, --json everywhere, documented exit codes.
package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nodelogicau/episodes-cli/internal/apperr"
	"github.com/nodelogicau/episodes-cli/internal/model"
	"github.com/nodelogicau/episodes-cli/internal/store"
)

// version is set at build time via -ldflags "-X .../internal/cli.version=...".
var version = "dev"

// ExitError carries an exit code and a machine-readable error code.
type ExitError = apperr.Error

var (
	usageErr    = apperr.Usage
	notFoundErr = apperr.NotFound
	refusedErr  = apperr.Refused
	dupErr      = apperr.Duplicate
	classify    = apperr.Classify
)

// app holds per-invocation state shared by all commands.
type app struct {
	jsonOut         bool
	workspace       string
	stdin           io.Reader
	stdout          io.Writer
	stderr          io.Writer
	stdinIsTerminal func() bool
	// warnings collects advisory notes raised while a verb runs. emit carries
	// them in the JSON result and prints them to stderr in text mode.
	warnings []string
}

// Execute runs the CLI with the given arguments and streams, returning the
// process exit code. It never calls os.Exit, so tests can drive it in-process.
func Execute(args []string, stdin io.Reader, stdout, stderr io.Writer, stdinIsTerminal func() bool) int {
	a := &app{stdin: stdin, stdout: stdout, stderr: stderr, stdinIsTerminal: stdinIsTerminal}
	root := a.rootCmd()
	root.SetArgs(args)
	root.SetIn(stdin)
	root.SetOut(stdout)
	root.SetErr(stderr)

	err := root.Execute()
	if err == nil {
		return apperr.ExitOK
	}
	ee := classify(err)
	if !isTyped(err) {
		ee = &ExitError{Code: apperr.ExitUsage, ErrCode: "usage", Err: err}
	}
	if !errors.Is(ee.Err, errQuiet) {
		a.printError(ee)
	}
	return ee.Code
}

func isTyped(err error) bool {
	return apperr.IsDomain(err) || errors.Is(err, errRuntime) || errors.Is(err, errQuiet) || errors.Is(err, apperr.ErrNoWorkspace) || errors.Is(err, apperr.ErrNotFound)
}

// errRuntime wraps arbitrary core errors so Execute can tell them apart from
// cobra's own usage errors.
var errRuntime = errors.New("runtime")

// errQuiet marks a check verdict whose report has already been emitted: the
// exit code carries the verdict and no error envelope follows it.
var errQuiet = errors.New("check failed")

func checkFailed() error {
	return &ExitError{Code: apperr.ExitCheckFailed, ErrCode: "check_failed", Err: errQuiet}
}

// run wraps a command body so that every error leaving it is typed.
func (a *app) run(fn func(cmd *cobra.Command, args []string) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		err := fn(cmd, args)
		if err == nil || isTyped(err) {
			return err
		}
		var probs model.Problems
		switch {
		case errors.As(err, &probs):
			return refusedErr("refused: %s", probs.Error())
		case errors.Is(err, store.ErrSealedMonth):
			return refusedErr("%v", err)
		}
		return fmt.Errorf("%w: %v", errRuntime, err)
	}
}

func (a *app) printError(ee *ExitError) {
	msg := strings.TrimPrefix(ee.Err.Error(), errRuntime.Error()+": ")
	if a.jsonOut {
		enc := json.NewEncoder(a.stderr)
		enc.SetEscapeHTML(false)
		_ = enc.Encode(map[string]any{"error": map[string]any{"code": ee.ErrCode, "message": msg}})
		return
	}
	fmt.Fprintf(a.stderr, "error: %s\n", msg)
}

// emit writes the command result: JSON when --json, otherwise via text.
func (a *app) emit(v map[string]any, text func(w io.Writer)) error {
	if len(a.warnings) > 0 {
		v["warnings"] = a.warnings
	}
	if a.jsonOut {
		enc := json.NewEncoder(a.stdout)
		enc.SetIndent("", "  ")
		enc.SetEscapeHTML(false)
		return enc.Encode(v)
	}
	text(a.stdout)
	for _, w := range a.warnings {
		fmt.Fprintf(a.stderr, "warning: %s\n", w)
	}
	return nil
}

func (a *app) openWorkspace() (*store.Workspace, error) { return store.Discover(a.workspace) }

func (a *app) rootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "episodes",
		Short: "Write and check Episodes Format workspaces: what happened, when, from whose standpoint",
		Long: `episodes reads and writes Episodes Format workspaces: directories of YAML
files, one per episode, recording what happened, when and where, from whose
standpoint, and where the record of it lives. Episodes accumulate and are
never revised: the only changes to an existing file are an observation of
its source (observe) and a redaction (redact).

It is built to be driven by an LLM harness and reviewed by people through
git: every verb is non-interactive, supports --json, performs no git
operations, and uses these exit codes:

  0 success   1 runtime error   2 usage error or refused write (incl. duplicate)
  3 not found 4 check failed    5 no workspace

Attribution on every writing verb: --author/--harness/--model, then
EPISODES_AUTHOR, EPISODES_HARNESS, EPISODES_MODEL, then defaults.source in
episodes.yaml. At least one of author or harness is required.`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().BoolVar(&a.jsonOut, "json", false, "emit a single JSON object on stdout (errors as JSON on stderr)")
	root.PersistentFlags().StringVar(&a.workspace, "workspace", "", "workspace directory (default: $"+store.EnvWorkspace+", else the nearest ancestor with episodes.yaml or a .episodes pointer)")
	root.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error { return usageErr("%v", err) })
	root.CompletionOptions.DisableDefaultCmd = true

	root.AddCommand(
		a.initCmd(),
		a.workspaceCmd(),
		a.recordCmd(),
		a.conversationCmd(),
		a.actCmd(),
		a.observeCmd(),
		a.redactCmd(),
		a.showCmd(),
		a.listCmd(),
		a.threadCmd(),
		a.validateCmd(),
		a.indexCmd(),
		a.versionCmd(),
	)
	return root
}

func (a *app) versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the binary version, the format it implements, and its link-relation snapshot date",
		Args:  cobra.NoArgs,
		RunE: a.run(func(cmd *cobra.Command, args []string) error {
			return a.emit(map[string]any{"version": version, "format": model.Format, "link_relations": model.LinkRelationsDate}, func(w io.Writer) {
				fmt.Fprintf(w, "episodes %s (%s; IANA link relations as of %s)\n", version, model.Format, model.LinkRelationsDate)
			})
		}),
	}
}
