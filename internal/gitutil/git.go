// Package gitutil wraps the small set of git operations aic needs.
//
// Everything shells out to the system `git` binary so behaviour matches what
// the user sees on the command line (hooks, config, credentials, etc.).
package gitutil

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// run executes git with the given args and returns trimmed stdout.
// On failure it returns an error that includes git's stderr.
func run(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return strings.TrimRight(stdout.String(), "\n"), nil
}

// runPassthrough executes git inheriting the parent's stdio so the user sees
// progress directly (used for commit/push/tag where live output is useful).
func runPassthrough(args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return nil
}

// EnsureRepo returns an error if the current directory is not inside a git
// work tree.
func EnsureRepo() error {
	out, err := run("rev-parse", "--is-inside-work-tree")
	if err != nil {
		return fmt.Errorf("not a git repository (or git is unavailable): %w", err)
	}
	if strings.TrimSpace(out) != "true" {
		return fmt.Errorf("not inside a git work tree")
	}
	return nil
}

// StagedDiff returns the diff of staged changes (git diff --cached).
// The result is empty when nothing is staged.
func StagedDiff() (string, error) {
	return run("diff", "--cached")
}

// StagedFiles returns the list of staged file paths (name + status).
func StagedFiles() (string, error) {
	return run("diff", "--cached", "--name-status")
}

// RecentCommitSubjects returns up to n recent commit subject lines, newest
// first. A fresh repository with no commits yields an empty slice, not an
// error.
func RecentCommitSubjects(n int) ([]string, error) {
	if n <= 0 {
		return nil, nil
	}
	out, err := run("log", fmt.Sprintf("-n%d", n), "--no-merges", "--pretty=format:%s")
	if err != nil {
		// A repo with no commits yet returns a non-zero exit; treat as empty.
		if strings.Contains(err.Error(), "does not have any commits") ||
			strings.Contains(err.Error(), "ambiguous argument 'HEAD'") {
			return nil, nil
		}
		return nil, err
	}
	if strings.TrimSpace(out) == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}

// CurrentBranch returns the current branch name, or "HEAD" when detached.
func CurrentBranch() (string, error) {
	return run("rev-parse", "--abbrev-ref", "HEAD")
}

// Commit creates a commit with the given message.
func Commit(message string) error {
	return runPassthrough("commit", "-m", message)
}

// Push pushes the current branch to its upstream. When the branch has no
// upstream configured it falls back to `git push origin <branch>` so the first
// push works without manual setup.
func Push() error {
	if err := runPassthrough("push"); err != nil {
		branch, berr := CurrentBranch()
		if berr != nil || branch == "" || branch == "HEAD" {
			return err
		}
		return runPassthrough("push", "--set-upstream", "origin", branch)
	}
	return nil
}

// CreateTag creates an annotated tag pointing at HEAD.
func CreateTag(name, message string) error {
	if message == "" {
		message = name
	}
	return runPassthrough("tag", "-a", name, "-m", message)
}

// PushTag pushes a single tag to origin.
func PushTag(name string) error {
	return runPassthrough("push", "origin", name)
}
