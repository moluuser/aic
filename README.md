# aic

Generate a git commit message from your **staged changes** using a local or
remote LLM — and optionally commit, push, and tag in one step.

`aic` reads your staged diff, your current branch, and your recent commit
history, then asks a model to write a commit message that matches your
project's existing style. It defaults to a **local Ollama** model so nothing
leaves your machine, and also ships with **OpenRouter** for hosted models —
other providers (OpenAI, Anthropic, LM Studio, vLLM, …) can be added without
touching the rest of the tool.

## Features

- Reads the staged diff (`git diff --cached`) — never the working tree.
- Reads the last N commit subjects (default 20) so the message matches your
  team's conventions (Conventional Commits or whatever you actually use).
- Includes the current branch name as context.
- **Prints only the message by default** — fully scriptable, does not commit.
- `-c` to commit, `-p` to commit + push, `--tag` to also create and push a tag.
- Strips `<think>…</think>` reasoning blocks and stray markdown fences from
  model output automatically.
- Single static binary; runs on Apple Silicon and Intel Macs.

## Install

Requires Go 1.21+ to build, and [Ollama](https://ollama.com) for the default
provider.

```sh
make install          # installs `aic` into your $GOBIN
# or
make build            # produces ./bin/aic
# or, for distributable macOS binaries (arm64, amd64, universal):
make dist
```

Make sure Ollama is running and you have a model pulled:

```sh
ollama serve &        # if not already running
ollama pull llama3    # or qwen2, deepseek-r1, etc.
```

## Usage

```sh
aic                       # print a suggested commit message
aic -c                    # generate and commit
aic -p                    # generate, commit, and push the branch
aic --tag v1.4.0          # generate, commit, push, then create+push tag
aic -m qwen2:latest -n 30 # specific model, 30 commits of history
aic --lang zh             # write the message in Chinese
```

When you use `-c`/`-p`/`--tag` interactively, `aic` shows the message and
asks for confirmation. Pass `-y` to skip the prompt (useful in scripts), or
`--dry-run` to see what would happen without changing anything.

Because the default mode writes only the message to stdout, you can pipe it:

```sh
git commit -m "$(aic)"
aic | pbcopy
```

### Flags

| Flag | Description |
| --- | --- |
| `-c, --commit` | create the commit using the generated message |
| `-p, --push` | commit, then push the current branch (implies `-c`) |
| `--tag <name>` | create an annotated tag at the new commit and push it (implies `-p`) |
| `-m, --model <name>` | model to use (overrides config) |
| `--provider <id>` | provider to use: `ollama`, `openrouter` |
| `-n, --history <N>` | recent commits to read for style (default 20) |
| `--endpoint <url>` | Ollama endpoint (default `http://localhost:11434`) |
| `--max-diff <N>` | max bytes of diff sent to the model (default 12000) |
| `--lang <code>` | language for the message, e.g. `en`, `zh`, `ja` |
| `-y, --yes` | skip the confirmation prompt before committing |
| `--dry-run` | with `-c`/`-p`, show what would happen without doing it |
| `-q, --quiet` | suppress progress messages on stderr |
| `--config <path>` | path to a config file |
| `--init` | write a default config file and exit |
| `--version` | print version |

## Configuration

`aic` reads `~/.config/aic/config.json` (or
`$XDG_CONFIG_HOME/aic/config.json`). Generate a starting point with:

```sh
aic --init
```

```json
{
  "provider": "ollama",
  "model": "llama3:latest",
  "history": 20,
  "max_diff_bytes": 12000,
  "language": "en",
  "extra_instructions": "",
  "ollama": {
    "endpoint": "http://localhost:11434"
  },
  "openrouter": {
    "endpoint": "https://openrouter.ai/api/v1",
    "api_key": ""
  }
}
```

`extra_instructions` is appended verbatim to the system prompt — use it to
encode project-specific commit conventions (e.g. "always reference the Jira
ticket from the branch name").

Precedence: command-line flags override the config file, which overrides the
built-in defaults.

### Using OpenRouter

[OpenRouter](https://openrouter.ai) gives you one API for hosted models from
many vendors. Set the provider and a model slug, and provide your API key:

```sh
export OPENROUTER_API_KEY=sk-or-...
aic --provider openrouter -m anthropic/claude-3.5-sonnet
```

The API key is read from `openrouter.api_key` in the config file, falling back
to the `OPENROUTER_API_KEY` environment variable. Prefer the environment
variable so your key never lands in a config file. To make OpenRouter the
default, set `"provider": "openrouter"` and a `"model"` in your config.

## Adding a provider

The LLM backend is behind a small interface in `internal/provider`:

```go
type Provider interface {
	Name() string
	Generate(ctx context.Context, req Request) (string, error)
}
```

To add OpenAI, Anthropic, LM Studio, vLLM, etc.: implement that interface in a
new file under `internal/provider`, then add a `case` to `provider.New`. The
rest of the tool — git handling, prompt building, output cleaning, the CLI —
stays unchanged.

## How it works

1. Verify the current directory is a git work tree.
2. `git diff --cached` for the staged diff (errors out if nothing is staged).
3. `git log -n N --pretty=format:%s` for recent commit subjects.
4. `git rev-parse --abbrev-ref HEAD` for the branch.
5. Build a system + user prompt and call the provider.
6. Clean the response and print it, or commit / push / tag as requested.
