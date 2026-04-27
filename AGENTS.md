# TeaForge AGENTS.md

You are working on TeaForge, a local agentic coding assistant written in Go.

## Development Rules

- Prefer small, composable changes over large rewrites.
- Preserve existing behavior unless the task explicitly changes behavior.
- Keep prompt/context pipeline changes covered by tests.
- Use session logging as the source of truth for prompt and turn auditing.

## Build and Test

- Build: `go build ./...`
- Test: `go test ./...`

## Prompt Pipeline Notes

- Prompt assembly lives in `internal/prompt`.
- System context is source-driven and assembled in source order.
- Guardrails run after message assembly and before model send.
