# tok

> Cut LLM token usage 60–90% — automatically.

```
$ tok prove git log -20
  RAW    1 842 tokens  (6 448 chars)
  tok      211 tokens  (  740 chars)  -89% saved
```

Three layers. Install once.

```
Layer 1  Shell output filtering    tok git log / tok docker ps / tok go test
Layer 2  Skill injection           brevity + KV-cache hints → ~20% fewer response tokens
Layer 3  Hook auto-rewriting       tok init -g installs PreToolUse hook — no manual prefix
```

---

## Install

```bash
go install github.com/gbudjeakp/tok@latest
tok init -g --copilot    # VS Code Copilot
tok init -g              # Claude Code
```

Reload VS Code / restart Claude after `tok init`.

---

## Commands

| Command | What it does |
|---|---|
| `tok <cmd> [args]` | Run any command through the filter engine |
| `tok prove <cmd>` | Show raw vs filtered output with token counts |
| `tok gain` | Savings dashboard across all sessions |
| `tok bench` | Benchmark tok vs raw (and rtk if installed) |
| `tok check <cmd>` | Dry-run: show which filter would match |
| `tok waste [--days N]` | Find unfiltered commands leaking tokens |
| `tok learn [--global]` | Mine transcripts for failure patterns → `CLAUDE.local.md` |
| `tok lockdeps [path]` | Compact lock file summary (package-lock, go.sum, Cargo.lock) |
| `tok session` | Transcript adoption report |
| `tok mode [lite\|full\|ultra\|off]` | Set skill injection intensity (no arg: show current) |
| `tok board [--no-open]` | Local HTML dashboard — tokens saved per day, streak, top commands |
| `tok init -g [--copilot]` | Install hook + inject skill |

`tok learn` also auto-fires in the background every 24h.

---

## Filter coverage

| Category | Commands |
|---|---|
| Git | `status`, `log`, `diff`, `add`, `commit`, `push`, `pull` |
| Tests | `go test`, `cargo test`, `pytest`, `jest`, `vitest` |
| Files | `ls`, `find`, `grep`, `rg` |
| Containers | `docker ps/images/logs`, `kubectl`, `oc` |
| Linters | `ruff`, `eslint`, `tsc`, `golangci-lint`, `biome`, `oxlint` |
| Build | `cargo build/check/clippy`, `go build/vet` |
| Packages | `pip`, `npm`, `yarn`, `pnpm`, `bun`, `brew`, `bundle` |
| Cloud | `aws` (ec2/s3/iam/lambda/sts), `gh` (pr/run), `pulumi`, `terraform`/`tofu` |
| JSON fallback | Any command returning JSON — arrays → compact table, objects → `key: val` |
| Read guard | Lock files, minified files, `node_modules/`, files >512KB — blocked from `read_file` |

Runner prefixes (`uv run`, `npx`, `poetry run`, `bunx`, …) and absolute paths (`/usr/bin/git`) are resolved automatically.

---

## vs. rtk

tok is a superset. [rtk](https://github.com/rtk-ai/rtk) has ~8 filters and requires a manual prefix. tok has 25+ filters, a PreToolUse hook (no prefix needed), session cache, read guard, and waste analysis. You cannot run both — they double-execute every command.

```bash
# Migrate from rtk:
go install github.com/gbudjeakp/tok@latest && tok init -g --copilot
rm ~/.copilot/hooks/rtk-rewrite.json  # remove rtk hook if present
```

