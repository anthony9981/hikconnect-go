# Commit Rules

## Hard rules

- **No attribution footers.** Never add `Co-Authored-By`, `Generated with`,
  `AI-assisted`, agent/bot names, or any tooling credit. Human commits only.
- **No secrets in commits.** `.env`, `sdk/`, `*.so` stay gitignored. Check
  `git diff --cached` for credentials before committing.
- **Never commit to rewrite history that's already pushed.** Local fixups
  fine; published history immutable.

## Message format

Conventional-style subject, imperative, ≤72 chars:

```
type: short summary
```

Types: `add`, `fix`, `update`, `refactor`, `docs`, `chore`, `test`.

Body (optional): why > what, dash bullets, wrap identifiers in backticks.

## Examples

Good:

```
Add channel auto-detect from login DeviceInfo

- HIK_CHANNEL=0 now picks StartChannel/StartIPChannel instead of
  hardcoding 1 (NVR IP channels start at 33)
```

Bad:

```
feat: add stuff (Generated with X)

Co-Authored-By: Some Bot <bot@...>
```

## Flow

1. `git status` + `git diff` — know what's staged
2. commit focused changes — one concern per commit
3. `git log --format=%B` — verify no footers slipped in
4. no remote configured yet — user adds it manually
