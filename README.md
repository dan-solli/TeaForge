# TeaForge

TeaForge is a local, terminal-first coding assistant built on Ollama. It combines an agentic tool loop, persistent memory, and a tree-sitter code index in a Bubble Tea TUI.

## What It Does

- Runs a tool-using coding agent in your terminal.
- Streams assistant output in real time.
- Persists project notes and session logs.
- Indexes code symbols for structural code search.
- Supports resumable sessions.

## Quick Start

### 1. Prerequisites

- Go 1.24+
- Ollama installed and running (`ollama serve`)
- C toolchain for CGO/tree-sitter (clang or gcc)

### 2. Install

Option A, from source:

```bash
git clone https://github.com/dan-solli/TeaForge
cd TeaForge
go build -o teaforge ./cmd/teaforge
```

Option B, prebuilt binaries from GitHub Releases:

- macOS: `arm64`, `amd64`
- Linux: `amd64`, `arm64`
- Windows: `amd64`

### 3. Start Ollama and Run

```bash
# Start Ollama (if not already running)
ollama serve

# Optional: pull a model first
ollama pull gemma4:26b

# Run TeaForge in your current project directory
./teaforge
```

## CLI Usage

### Flags

```bash
# Print build metadata and exit
./teaforge --version

# Resume a specific session ID (filename without .json)
./teaforge --resume 2026-04-28T04-47-00Z

# Resume the newest saved session
./teaforge --resume-latest
```

### Environment Variables

| Variable | Purpose | Default |
|---|---|---|
| `TEAFORGE_MODEL` | Ollama model name | `gemma4:26b` |
| `OLLAMA_HOST` | Ollama base URL | `http://localhost:11434` |
| `TEAFORGE_NUM_CTX` | Override model context length | auto-detected from Ollama model metadata |
| `TEAFORGE_CTX_USAGE_PERCENT` | Percent of context used for prompt assembly | `80` |

Notes:

- If `TEAFORGE_NUM_CTX` is unset, TeaForge queries Ollama for the active model context length.
- TeaForge uses only a percentage of that context for prompt assembly (default 80%) to preserve headroom for model output and tool-call responses.

Examples:

```bash
TEAFORGE_MODEL=qwen2.5-coder:14b ./teaforge
OLLAMA_HOST=http://192.168.1.100:11434 ./teaforge
TEAFORGE_NUM_CTX=262144 ./teaforge
TEAFORGE_CTX_USAGE_PERCENT=75 ./teaforge
```

## TUI Workflow

### Views

- Chat (`ctrl+1`): interact with the agent.
- Files (`ctrl+2`): browse files and attach file(s) for the next chat turn.
- Memory (`ctrl+3`): inspect session/project/code memory and symbol search.
- Models (`ctrl+4`): list and switch installed Ollama models.

### Typical Flow

1. Open Files view and press Enter on one or more files to queue attachments.
2. Return to Chat and submit your prompt.
3. Agent processes your prompt with attached files and available tools.
4. Use Memory view to inspect saved notes and indexed symbols.

## Key Bindings

| Key | Action |
|---|---|
| `ctrl+1` `ctrl+2` `ctrl+3` `ctrl+4` | Switch views |
| `enter` (Chat) | Send a single-line message |
| `ctrl+s` (Chat) | Send multiline message |
| `enter` (Files) | Attach selected file for next turn |
| `enter` (Models) | Select active model |
| `ctrl+n` | Start new session |
| `ctrl+r` | Open session picker (resume) |
| `ctrl+shift+r` | Re-index code memory |
| `tab` / `shift+tab` | Memory tab navigation |
| `/` (Memory) | Start symbol search |
| `up` / `down` | Navigate lists |
| `ctrl+c` / `ctrl+q` | Quit |

## Memory and Storage

TeaForge stores project-scoped data under `.teaforge/` in your working directory:

- `.teaforge/memory.json`: persistent project notes.
- `.teaforge/sessions/*.json`: session transcripts and summaries used for resume.

Memory layers:

- Session memory: live conversation context.
- Project memory: durable notes/decisions.
- Code memory: tree-sitter index of symbols.

## Built-in Agent Tools

| Tool | Purpose |
|---|---|
| `read_file` | Read file content |
| `write_file` | Create/overwrite files |
| `edit_file` | Exact-string surgical edits |
| `list_directory` | List directory contents |
| `run_command` | Run shell command (bounded output/timeout) |
| `save_note` | Persist project notes |
| `recall_notes` | Query saved notes |
| `list_note_categories` | Show note categories with counts |
| `search_code` | Search indexed code symbols |
| `index_directory` | Build/refresh tree-sitter code index |

## Development

```bash
go test ./...
go build -o teaforge ./cmd/teaforge
```

## Releases and Changelog

TeaForge uses Semantic Versioning (`vMAJOR.MINOR.PATCH`) and Release Please.

### Commit Convention

Use Conventional Commits:

- `feat:` -> minor release
- `fix:` / `perf:` -> patch release
- `BREAKING CHANGE:` -> major release

### Automated Release Flow

1. Push to `main`.
2. `Release Please` updates or opens a release PR.
3. Merge the release PR to create tag + GitHub Release and update `CHANGELOG.md`.
4. `Release Artifacts` builds archives and uploads release assets + `checksums.txt`.

### If Artifact Upload Does Not Auto-Trigger

Manually dispatch `Release Artifacts` with the release tag (for example `v0.2.0`) from GitHub Actions workflow dispatch.

## Project Layout

```text
cmd/teaforge/          CLI entrypoint
.github/workflows/     CI, commit lint, release automation
internal/
  agent/               Agent loop and memory-aware tools
  buildinfo/           Build/version metadata exposed by --version
  memory/              Session and project memory
  ollama/              Ollama API client
  prompt/              Prompt pipeline and guardrails
    templates/         Embedded prompt/template files
  tools/               Core built-in tools
  treesitter/          Code index and symbol extraction
  tui/                 Bubble Tea application and views
```

## License

See [LICENSE](LICENSE).
