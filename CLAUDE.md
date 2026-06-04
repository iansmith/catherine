# catherine — project rules

## Ticket / issue creation

**All GitHub issues (tickets) in this project MUST be created with the `/slopstop-create-gh` skill — and nothing else.**

That skill is the only mechanism that guarantees the ticket key equals the issue number (`CATH-N` = GitHub issue `#N`). The downstream slopstop skills (`/slopstop-start`, `/slopstop-pr`, `/slopstop-merge`, …) strip the digits from the key to resolve the issue number, so any issue created another way breaks that contract and has no valid ticket key.

Do **not** create issues via:
- `gh issue create`
- the GitHub MCP `issue_write` (method `create`) directly
- the GitHub web UI

If an issue is ever created outside `/slopstop-create-gh`, its number/key alignment is not guaranteed and it must be reconciled by hand before any `:start`/`:pr`/`:merge` skill is run against it.

Project config lives in `.project-conf.toml` (system `github`, key `iansmith/catherine`, prefix `CATH`).
