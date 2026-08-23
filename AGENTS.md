# Agent Development Guidelines

This file is intended for AI coding assistants (Gemini, Codex, Claude Code, etc.) that are modifying or extending RunStack. 
**You MUST read this file before making any architectural or codebase changes.**

## Architecture Rules

- **Control Plane owns state:** The Control Plane is the absolute authority over job and node state.
- **Agents never directly modify Control Plane state:** Agents must interact via defined HTTP endpoints (`/claim`, `/result`).
- **Scheduler assigns, but does not execute:** The scheduler transitions jobs from `PENDING` to `ASSIGNED` but has no awareness of command execution.
- **Agents execute, but do not schedule:** Agents pull `ASSIGNED` jobs assigned to their explicit Node ID, claim them, execute them locally, and report results.
- **Domain rules belong inside internal packages:** State transition logic and validation (e.g., in `internal/job` and `internal/node`) must stay fully decoupled from the API layer.
- **HTTP handlers should remain thin:** Handlers exist only to parse requests, invoke domain logic, and serialize responses.
- **Registries are responsible for concurrency safety:** Data races are prevented by strict `sync.RWMutex` usage within Registry structs.

## Development Rules

Always run the following command to validate your changes:
```bash
make check
```

- **Never** commit generated binaries or logs.
- **Never** introduce a third-party dependency without explicit justification and user approval.
- **Do not** rewrite existing architecture without an explicit reason.
- **Do not** silently change domain state transitions or bypass them in the API.

## AI Agent Workflow

Before modifying code:
1. Read `AGENTS.md`.
2. Read `MEMORY.md` to understand the current state and limitations.
3. Inspect the existing architecture.
4. Identify the affected package boundaries.
5. Make the smallest architectural change necessary.
6. Add or update tests (unit, race, and integration).
7. Run the complete validation suite.
8. Report modified files and architectural impact.
9. **Never commit unless explicitly requested by the user.**

## Documentation Synchronization

Any architectural change MUST update the relevant documentation:
- New subsystem → `docs/ARCHITECTURE.md`
- New capability → `SKILLS.md`
- New milestone or future plan → `docs/ROADMAP.md`
- Important implementation decision → `MEMORY.md`
- New developer workflow → `docs/DEVELOPMENT.md`
- Public API change → `docs/API.md`
