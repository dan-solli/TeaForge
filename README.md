# TeaForge

> A TUI-based agentic software development environment powered by local Ollama AI models.

TeaForge puts an AI coding assistant directly in your terminal. It follows the agentic patterns established by Anthropic, combining a streaming LLM loop with built-in tools, three layers of memory, and tree-sitter powered code analysis.

---

## Features

### 🤖 Agentic Loop
- Implements the Anthropic-style tool-use agent loop: the model reasons, calls tools, observes results, and continues until the task is complete.
- Streams tokens to the TUI in real time.

### 🧠 Three Memory Layers
| Layer | What it stores | Persistence |
|---|---|---|
| **Session memory** | The current conversation history and key-value context | In-process only |
| **Project memory** | Decisions, notes and discoveries (saved as `/.teaforge/memory.json`) | Persisted to disk |
| **Code memory** | Structural index of the source tree built with tree-sitter | Rebuilt on start / on demand |

### 🛠 Built-in Tools
The agent can call these tools autonomously:

| Tool | Description |
|---|---|
| `read_file` | Read any file from the filesystem |
| `write_file` | Create or overwrite a file |
| `edit_file` | Replace an exact string in a file (surgical edits) |
| `list_directory` | List files in a directory |
| `run_command` | Execute a shell command (60 s timeout) |
| `search_code` | Full-text symbol search across the tree-sitter index |
| `index_directory` | (Re-)index a directory into the code memory |
| `save_note` | Persist a project note to disk |

### 🌳 Tree-sitter Code Analysis
Supported languages: **Go**, **Python**, **TypeScript**, **JavaScript**.

TeaForge indexes your project on startup, extracting functions, types, constants and imports so the AI always has structural context available via `search_code`.

### 💬 TUI Views
- **Chat** (`ctrl+1`) – streaming conversation with the agent
- **Files** (`ctrl+2`) – file tree explorer; press Enter on a file to summarise it
- **Memory** (`ctrl+3`) – browse all three memory layers; press `/` to search code symbols
- **Models** (`ctrl+4`) – list and switch between installed Ollama models

---

## Requirements
- [Go 1.21+](https://go.dev/dl/)
- [Ollama](https://ollama.com/) running locally (`ollama serve`)
- A C compiler (for tree-sitter CGO bindings) — GCC or Clang

## Installation

```bash
git clone https://github.com/dan-solli/TeaForge
cd TeaForge
go build -o teaforge ./cmd/teaforge
```

## Running

```bash
# Use the default model (llama3.2) and current directory
./teaforge

# Choose a different model
TEAFORGE_MODEL=codellama ./teaforge

# Point at a remote Ollama instance
OLLAMA_HOST=http://192.168.1.100:11434 ./teaforge
```

## Key Bindings

| Key | Action |
|---|---|
| `ctrl+1` – `ctrl+4` | Switch views |
| `Enter` | Send message (single line) / expand directory |
| `ctrl+s` | Send multi-line message |
| `ctrl+n` | Start a new session |
| `ctrl+r` | Re-index the working directory |
| `tab` / `shift+tab` | Cycle memory tabs |
| `/` | Search code symbols (in Memory view) |
| `↑` / `↓` | Navigate lists |
| `ctrl+c` | Quit |

## Project Layout

```
cmd/teaforge/          Entry point
internal/
  agent/               Anthropic-style agentic loop + memory-aware tools
  memory/              Session, project and code memory
  ollama/              Ollama REST API client (streaming chat)
  tools/               Built-in tools (file I/O, command runner)
  treesitter/          Tree-sitter code index (code memory)
  tui/                 Bubble Tea TUI
    styles/            Visual styling constants
    views/             Chat, Files and Memory view components
```

## License

See [LICENSE](LICENSE).
