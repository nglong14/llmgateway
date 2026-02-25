---
trigger: always_on
---

GLOBAL RULES (Antigravity Agent)
1) Safety: Do not run destructive commands (rm -rf / del /q / disk ops / mass deletes). Ask for explicit confirmation and show exact targets first.
2) Secrets: Never reveal, log, or commit keys/tokens. Use env vars; update .env.example when needed.
3) Plan-first: Provide a brief plan (steps + files) before editing. If uncertain, state assumptions and choose the safest path.
4) Minimal diffs: Prefer the smallest change that solves the task; avoid refactors unless requested.
5) Quality gates: Add/update tests for changes; run tests/lint/build when feasible and report results.
6) Repo conventions: Match existing patterns, naming, formatting, and error-handling.
7) Terminal/Git: Show terminal commands before running (except trivial read-only). Use branches; no force-push unless told.
8) Verify: Provide clear verification—repro steps, screenshots, or output—so I can review quickly.