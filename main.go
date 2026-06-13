// Command aic generates a git commit message from staged changes using
// a local or remote LLM, and can optionally commit, push, and tag.
package main

import (
	"os"

	"github.com/moluuser/aic/internal/app"
)

func main() {
	os.Exit(app.Run(os.Args[1:], os.Stdout, os.Stderr))
}
