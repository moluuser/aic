// Package app wires together flags, config, git, and the LLM provider to
// implement the aic command.
package app

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/moluuser/aic/internal/config"
	"github.com/moluuser/aic/internal/gitutil"
	"github.com/moluuser/aic/internal/prompt"
	"github.com/moluuser/aic/internal/provider"
)

// Version is overridden at build time via -ldflags.
var Version = "dev"

// options collects parsed command-line flags.
type options struct {
	commit     bool
	push       bool
	tag        string
	model      string
	provider   string
	history    int
	endpoint   string
	maxDiff    int
	lang       string
	configPath string
	yes        bool
	dryRun     bool
	quiet      bool
	showVer    bool
	initCfg    bool
}

const usage = `aic — generate a git commit message from staged changes using an LLM.

Usage:
  aic [flags]

By default it prints a generated commit message and does nothing else.

Flags:
  -c, --commit         create the commit using the generated message
  -p, --push           commit, then push the current branch (implies -c)
      --tag <name>     create an annotated tag at the new commit and push it
                       (implies -p)
  -m, --model <name>   model to use (overrides config)
      --provider <id>  provider to use: ollama, openrouter (default from config)
  -n, --history <N>    number of recent commits to read for style (default 20)
      --endpoint <url> Ollama endpoint (default http://localhost:11434)
      --max-diff <N>   max bytes of diff sent to the model (default 12000)
      --lang <code>    language for the commit message, e.g. en, zh, ja
  -y, --yes            skip the confirmation prompt before committing
      --dry-run        with -c/-p, show what would happen without doing it
  -q, --quiet          suppress progress messages on stderr
      --config <path>  path to config file
      --init           write a default config file and exit
      --version        print version and exit
  -h, --help           show this help

Examples:
  aic                       # print a suggested message
  aic -c                    # generate and commit
  aic -p                    # generate, commit, and push
  aic --tag v1.4.0          # generate, commit, push, tag, push tag
  aic -m qwen2:latest -n 30 # use a specific model and more history
`

func parseFlags(args []string, stderr io.Writer) (options, error) {
	var o options
	fs := flag.NewFlagSet("aic", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { fmt.Fprint(stderr, usage) }

	fs.BoolVar(&o.commit, "commit", false, "")
	fs.BoolVar(&o.commit, "c", false, "")
	fs.BoolVar(&o.push, "push", false, "")
	fs.BoolVar(&o.push, "p", false, "")
	fs.StringVar(&o.tag, "tag", "", "")
	fs.StringVar(&o.model, "model", "", "")
	fs.StringVar(&o.model, "m", "", "")
	fs.StringVar(&o.provider, "provider", "", "")
	fs.IntVar(&o.history, "history", -1, "")
	fs.IntVar(&o.history, "n", -1, "")
	fs.StringVar(&o.endpoint, "endpoint", "", "")
	fs.IntVar(&o.maxDiff, "max-diff", -1, "")
	fs.StringVar(&o.lang, "lang", "", "")
	fs.StringVar(&o.configPath, "config", "", "")
	fs.BoolVar(&o.yes, "yes", false, "")
	fs.BoolVar(&o.yes, "y", false, "")
	fs.BoolVar(&o.dryRun, "dry-run", false, "")
	fs.BoolVar(&o.quiet, "quiet", false, "")
	fs.BoolVar(&o.quiet, "q", false, "")
	fs.BoolVar(&o.showVer, "version", false, "")
	fs.BoolVar(&o.initCfg, "init", false, "")

	if err := fs.Parse(args); err != nil {
		return o, err
	}

	// Resolve implications: --tag implies push implies commit.
	if o.tag != "" {
		o.push = true
	}
	if o.push {
		o.commit = true
	}
	return o, nil
}

// Run executes the command and returns a process exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	o, err := parseFlags(args, stderr)
	if err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	if o.showVer {
		fmt.Fprintln(stdout, "aic", Version)
		return 0
	}

	logf := func(format string, a ...any) {
		if !o.quiet {
			fmt.Fprintf(stderr, format+"\n", a...)
		}
	}

	// Load config and overlay flag values.
	cfg, err := config.Load(o.configPath)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	applyFlags(&cfg, o)

	if o.initCfg {
		if err := config.Save(cfg, o.configPath); err != nil {
			fmt.Fprintln(stderr, "error:", err)
			return 1
		}
		path := o.configPath
		if path == "" {
			path = config.DefaultPath()
		}
		fmt.Fprintln(stdout, "wrote config to", path)
		return 0
	}

	if err := gitutil.EnsureRepo(); err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}

	message, err := generate(cfg, logf)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}

	if !o.commit {
		// Default mode: just print the message to stdout.
		fmt.Fprintln(stdout, message)
		return 0
	}

	return doCommit(o, message, stdout, stderr, logf)
}

// applyFlags overlays explicitly-set flag values onto cfg.
func applyFlags(cfg *config.Config, o options) {
	if o.provider != "" {
		cfg.Provider = o.provider
	}
	if o.model != "" {
		cfg.Model = o.model
	}
	if o.history >= 0 {
		cfg.History = o.history
	}
	if o.endpoint != "" {
		cfg.Ollama.Endpoint = o.endpoint
	}
	if o.maxDiff >= 0 {
		cfg.MaxDiffBytes = o.maxDiff
	}
	if o.lang != "" {
		cfg.Language = o.lang
	}
}

// generate builds the prompt from git context and calls the provider.
func generate(cfg config.Config, logf func(string, ...any)) (string, error) {
	diff, err := gitutil.StagedDiff()
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(diff) == "" {
		return "", fmt.Errorf("no staged changes; stage files with `git add` first")
	}

	files, _ := gitutil.StagedFiles()
	branch, _ := gitutil.CurrentBranch()
	history, err := gitutil.RecentCommitSubjects(cfg.History)
	if err != nil {
		return "", err
	}

	in := prompt.Input{
		Branch:            branch,
		StagedFiles:       files,
		Diff:              diff,
		History:           history,
		Language:          cfg.Language,
		ExtraInstructions: cfg.ExtraInstructions,
		MaxDiffBytes:      cfg.MaxDiffBytes,
	}

	p, err := provider.New(cfg)
	if err != nil {
		return "", err
	}

	logf("Generating commit message with %s (%s)...", p.Name(), cfg.Model)

	raw, err := p.Generate(context.Background(), provider.Request{
		System: in.System(),
		User:   in.User(),
		Model:  cfg.Model,
	})
	if err != nil {
		return "", err
	}

	message := prompt.Clean(raw)
	if message == "" {
		return "", fmt.Errorf("model returned an empty commit message")
	}
	return message, nil
}

// doCommit performs commit / push / tag according to o.
func doCommit(o options, message string, stdout, stderr io.Writer, logf func(string, ...any)) int {
	// Always show the message that will be used.
	fmt.Fprintln(stderr, "\n--- commit message ---")
	fmt.Fprintln(stderr, message)
	fmt.Fprintln(stderr, "----------------------")

	if o.dryRun {
		logf("[dry-run] would commit%s%s", pushNote(o), tagNote(o))
		return 0
	}

	if !o.yes && isTerminal(os.Stdin) {
		if !confirm(stderr, os.Stdin, "Commit with this message?") {
			logf("aborted")
			return 1
		}
	}

	if err := gitutil.Commit(message); err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	logf("committed")

	if o.push {
		if err := gitutil.Push(); err != nil {
			fmt.Fprintln(stderr, "error:", err)
			return 1
		}
		logf("pushed")
	}

	if o.tag != "" {
		if err := gitutil.CreateTag(o.tag, message); err != nil {
			fmt.Fprintln(stderr, "error:", err)
			return 1
		}
		if err := gitutil.PushTag(o.tag); err != nil {
			fmt.Fprintln(stderr, "error:", err)
			return 1
		}
		logf("tagged and pushed %s", o.tag)
	}

	return 0
}

func pushNote(o options) string {
	if o.push {
		return " and push"
	}
	return ""
}

func tagNote(o options) string {
	if o.tag != "" {
		return fmt.Sprintf(" and tag %s", o.tag)
	}
	return ""
}

// confirm asks a yes/no question on stderr and reads the answer from r.
func confirm(w io.Writer, r io.Reader, question string) bool {
	fmt.Fprintf(w, "%s [y/N] ", question)
	reader := bufio.NewReader(r)
	line, _ := reader.ReadString('\n')
	line = strings.ToLower(strings.TrimSpace(line))
	return line == "y" || line == "yes"
}

// isTerminal reports whether f is an interactive terminal.
func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
