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

### Updating

```bash
tok upgrade              # go install @latest, reports before/after version
tok doctor               # diagnose hook/skill/PATH health after upgrading
```

Tagged releases (`vX.Y.Z`) also publish prebuilt binaries for macOS/Linux/Windows
(amd64/arm64) to [GitHub Releases](https://github.com/gbudjeakp/tok/releases) via GoReleaser.

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
| `tok doctor` | Diagnose hook/skill/PATH health |
| `tok upgrade` | Update to the latest version |
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

## Techniques used (and why)

tok's approach draws on a few documented token-reduction strategies, adapted to stay dependency-free (no ML runtime, no vector DB):

- **Session cache with near-duplicate diffing** — exact repeats return a one-liner; outputs that are merely *similar* (e.g. one container restarted in `docker ps`) return only the changed lines via line-shingle Jaccard similarity. A lexical, embedding-free stand-in for the semantic caching idea in [GPTCache](https://github.com/zilliztech/GPTCache).
- **Cache-alignment skill guidance** — the injected skill block now spells out the exact anti-pattern [Anthropic's prompt caching docs](https://platform.claude.com/docs/en/build-with-claude/prompt-caching) call out as the most common mistake: putting per-turn variable content (timestamps, IDs) *before* the stable prefix breaks the provider's cache for every subsequent token. Provider prompt caching (Anthropic/OpenAI/Gemini) can cut cached-token cost by up to 90% — the highest-leverage lever available, and one tok can only nudge via instructions since it doesn't sit in the API path.
- **Signal-over-noise line filtering** — the per-tool filters (git, test runners, build tools) already keep error/fail/warn lines and drop progress noise; this is the same information-density idea behind [LLMLingua](https://github.com/microsoft/LLMLingua)'s prompt compression, done with keyword heuristics instead of a trained model (keeps tok a single static binary).
- **Log deduplication** — consecutive identical lines collapse to `[Nx] line`, the cheapest form of redundancy removal.

---

## vs. rtk

tok is a superset. [rtk](https://github.com/rtk-ai/rtk) has ~8 filters and requires a manual prefix. tok has 25+ filters, a PreToolUse hook (no prefix needed), session cache, read guard, and waste analysis. You cannot run both — they double-execute every command.

```bash
# Migrate from rtk:
go install github.com/gbudjeakp/tok@latest && tok init -g --copilot
rm ~/.copilot/hooks/rtk-rewrite.json  # remove rtk hook if present
```

