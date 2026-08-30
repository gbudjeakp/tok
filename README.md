# tok

> Cut LLM token usage by 60–90% — automatically.

`tok` reduces the tokens your AI agent consumes across three independent layers. Install once, and every terminal command the agent runs is silently filtered, compressed, and measured.

```
$ tok prove git log -20
╔══════════════════════════════════════════════════════════════╗
║  tok prove: git log -20                                      ║
╠══════════════════════════════════════════════════════════════╣
║  RAW    1 842 tokens  (6 448 chars)                          ║
║  tok      211 tokens  (  740 chars)  -89% saved              ║
╠══════════════════════════════════════════════════════════════╣
```

---

## How it works — three layers

```
┌─────────────────────────────────────────────────────────────┐
│  Layer 1 — Shell output filtering                           │
│  tok git log / tok docker ps / tok go test ./...            │
│  Compresses noisy CLI output before it enters the context.  │
├─────────────────────────────────────────────────────────────┤
│  Layer 2 — Skill injection  (tok init)                      │
│  Injects brevity + KV-cache-alignment hints into            │
│  CLAUDE.md / copilot-instructions.md so the model writes    │
│  shorter responses and keeps stable content at the front.   │
├─────────────────────────────────────────────────────────────┤
│  Layer 3 — Hook auto-rewriting  (tok init -g)               │
│  Installs a PreToolUse hook so the agent rewrites           │
│  run_in_terminal / Bash calls to tok equivalents            │
│  automatically — no manual prefix required.                 │
└─────────────────────────────────────────────────────────────┘
```

After `tok init -g --copilot`, every command your agent runs is silently proxied through Layer 1. You never type `tok` yourself.

---

## Install

```bash
go install github.com/you/tok@latest
tok init -g --copilot    # VS Code Copilot
# or
tok init -g              # Claude Code
```

`tok init` does three things:

1. Writes `~/.copilot/hooks/tok.json` (or `~/.claude/settings.json`) — the PreToolUse hook that auto-rewrites commands.
2. Injects the tok skill block into your agent's instruction file.
3. Adds itself to PATH (via `go install`).

Reload VS Code (or restart Claude Code) for the hook to take effect.

---

## Commands

### `tok <command> [args]`

Run any command through the filter engine directly. Useful for testing, for agents that don't support hooks, or for piping.

```bash
tok git status           # branch | modified(3): foo.go bar.go | untracked(1): baz
tok git log -20          # abc123 feat: add user auth (Alice, 2d ago) …
tok docker ps            # ID  image  status  ports — 1 container
tok go test ./...        # --- FAIL: TestLogin (0.45s) … FAIL (12 tests, 2 failed)
tok aws ec2 describe-instances   # compact JSON, stripped type wrappers
```

Savings are recorded to `~/.local/share/tok/history.ndjson` automatically.

### `tok prove <command> [args]`

Runs the command twice — once raw, once through tok — and shows both outputs with token counts side-by-side. Use this to see savings in real time or to benchmark against other tools.

```
$ tok prove git status
╔══════════════════════════════════════════════════════════════╗
║  tok prove: git status                                       ║
╠══════════════════════════════════════════════════════════════╣
║  RAW    67 tokens  (234 chars)                               ║
║  tok    13 tokens  ( 45 chars)  -80% saved                   ║
╠══════════════════════════════════════════════════════════════╣
║  RAW OUTPUT                                                  ║
╟──────────────────────────────────────────────────────────────╢
║  On branch main                                              ║
║                                                              ║
║  Changes not staged for commit:                              ║
║    (use "git add <file>..." to update what will be committed) ║
║       modified:   src/auth.ts                                ║
║       modified:   src/db.ts                                  ║
╠══════════════════════════════════════════════════════════════╣
║  tok OUTPUT                                                  ║
╟──────────────────────────────────────────────────────────────╢
║  main | modified(2): src/auth.ts, src/db.ts                  ║
╚══════════════════════════════════════════════════════════════╝
```

### `tok gain`

Unified savings dashboard across all recorded sessions.

```bash
tok gain            # last 30 days
tok gain --history  # per-command breakdown
```

### `tok session`

Reads your VS Code Copilot transcripts and reports what fraction of terminal commands went through tok vs. raw. Shows which commands you're missing savings on.

```
Analysed 8 sessions (last 7 days)

tok adoption:  72%  (1 698 / 2 352 terminal commands filtered)

Missed opportunities:
  aws   41 calls  ~30–60% savings
  grep  18 calls  ~40–70% savings
```

### `tok learn`

Mines recent session transcripts for repeated failure patterns — commands the agent ran dozens of times, polling loops, error → retry cycles — and writes a concise digest to `CLAUDE.local.md`. Agents that read this file avoid repeating the same mistakes.

```bash
tok learn              # write patterns to ./CLAUDE.local.md
tok learn --global     # write to ~/.claude/CLAUDE.local.md
tok learn --dry-run    # print without writing
tok learn --days 14    # look back further
```

Example output:

```
Scanning 8 sessions from last 7 days…
Wrote 4 lesson(s) to ./CLAUDE.local.md

1. sleep  (seen 4 times) — run 79 times in one session: sleep 5 && aws ssm get-command-invocation ...
2. aws    (seen 3 times) — run 80 times: aws ec2 terminate-instances ...
```

The patterns are written as a Markdown block under a `<!-- tok-learn: start -->` marker. Running `tok learn` again replaces the block rather than appending.

### `tok check`

Confirm which filter would fire for a command — without running it. Useful when using runner prefixes or absolute paths.

```bash
tok check git log -20          # filtered   git log -20
tok check uv run pytest -v     # filtered   uv run pytest -v  (resolved → pytest)
tok check /usr/bin/docker ps   # filtered   /usr/bin/docker ps  (resolved → docker)
tok check curl -v example.com  # no filter  curl -v example.com
```

### `tok waste`

Scan recent VS Code Copilot transcripts and show which commands are leaking the most tokens — unfiltered calls and polling loops.

```bash
tok waste                # last 7 days
tok waste --days 30      # wider window
```

Example output:

```
Waste report — last 7 days (8 sessions, 2455 terminal calls)

command                  calls   ~tokens   repeats
------------------------------------------------------
aws                        299    209.3k  ⚠ 2 sessions polled 3+×
git                        223    178.4k  ⚠ 3 sessions polled 3+×
pnpm                       182    109.2k  ⚠ 4 sessions polled 3+×
------------------------------------------------------
TOTAL (estimated)                 849.5k
```

### `tok lockdeps`

Show a compact name/version table from a lock file. Called automatically by the read guard when Copilot tries to read a full `package-lock.json`, `Cargo.lock`, or `go.sum`.

```bash
tok lockdeps              # auto-detect in cwd
tok lockdeps go.sum       # specific file
```


### `tok init`

```bash
tok init -g                  # Claude Code: hook + ~/.claude/CLAUDE.md skill
tok init -g --copilot        # VS Code Copilot: hook + ~/.copilot/copilot-instructions.md skill
tok init --uninstall         # remove hook and skill block
tok init --uninstall --copilot
```

---

## Filter coverage

| Category | Commands |
|---|---|
| Git | `git status`, `git log`, `git diff`, `git add`, `git commit`, `git push`, `git pull` |
| Tests | `go test`, `cargo test`, `pytest`, `npm test` / `jest` |
| Files | `ls`, `find`, `grep`, `rg` |
| Containers | `docker ps`, `docker images`, `docker logs`, `kubectl`, `oc` |
| Linters | `ruff`, `eslint`, `tsc`, `golangci-lint`, `biome`, `oxlint` |
| Build | `cargo build/check/clippy`, `go build/vet` |
| Packages | `pip`, `npm`, `yarn`, `pnpm`, `bun`, `brew`, `bundle` |
| Cloud | `aws` (ec2/s3/iam/lambda/sts), `gh` (pr/run), `pulumi`, `terraform`/`tofu` |
| JSON fallback | Any unrecognized subcommand that returns JSON — arrays compressed to table headers + compact rows, objects to `key: value` pairs |
| Read guard | Lock files, minified files, `node_modules/`, `dist/` — blocked before entering context; bounded reads always allowed |

**Runner prefix stripping:** tok automatically unwraps `uv run`, `poetry run`, `npx`, `bunx`, `pnpm exec`, and others — `uv run pytest -v` applies the pytest filter transparently.

**Absolute path resolution:** `/usr/bin/git` and `/opt/homebrew/bin/docker` resolve to their base command for filter matching.

**Session memory cache:** If the same command produces the same output within its TTL (30s for git/ls, 5min for build tools), tok returns a one-liner `[unchanged: same N-char output as Xs ago]` instead of the full text.

**Log deduplication:** Consecutive identical lines are collapsed to `[Nx] line` before output reaches the model.

---

## Comparison with similar tools

### vs. rtk

[rtk](https://github.com/rtk-ai/rtk) is the closest relative — a shell output filter with a token savings dashboard. Core differences:

| | rtk | tok |
|---|---|---|
| Filter count | ~8 | 25+ |
| Auto-rewrites (hook) | ❌ manual prefix always | ✅ hook fires automatically after `tok init` |
| Skill injection | ❌ | ✅ brevity + cache alignment |
| Analytics | ✅ savings dashboard | ✅ savings dashboard + `tok prove` live comparison |
| Agent support | Claude Code | Claude Code + VS Code Copilot |

You cannot stack rtk and tok — they do the same thing and would double-execute commands. Pick one. tok is a superset.

### vs. caveman

[caveman](https://github.com/juliusbrussee/caveman) is purely a skill — a text block you paste into `CLAUDE.md` that instructs the model to be terse. It is Layer 2 only.

tok includes the same skill (and auto-injects it via `tok init`), plus adds Layers 1 and 3. If you're already using caveman's instruction block, `tok init` will replace it with tok's version — they're compatible in intent but you only need one.

**You can stack caveman + tok only if you keep caveman's block out of the file tok manages.** There is no benefit to running both.

### vs. headroom

[headroom](https://github.com/headroomlabs-ai/headroom) is an API proxy: it intercepts the raw HTTP stream between your agent and the model provider, applies ML-based compression, and handles cache alignment at the network layer. Completely different architecture.

| | headroom | tok |
|---|---|---|
| Approach | API proxy (intercepts HTTP) | CLI binary (filters shell output) |
| Setup | Run a local server, point agent at proxy | `go install && tok init` |
| Coverage | Every model request/response | Shell command output only |
| Latency | Small proxy overhead | Zero (filtering happens during execution) |
| Compression | ML-based (Kompress-v2, CodeCompressor) | Rule-based + JSON SmashCrusher |
| Session learning | `headroom learn` — ML-extracted lessons | `tok learn` — pattern-mined lessons |
| Analytics | Built-in | `tok gain`, `tok prove` |

What headroom has that tok doesn't: ML-based compression (works on arbitrary content — not just shell output), CCR reversible compression with retrieval, and CodeCompressor (AST-aware code compression). These require Python 3.10+ and an ONNX model download.

**These are complementary, not competing.** headroom compresses all content at the API level; tok compresses the specific slice of content that comes from shell commands. Using both would add savings at different layers, but headroom's setup cost is non-trivial.

### Benchmark — `tok prove` as ground truth

Rather than synthetic benchmarks, use `tok prove` on your actual commands:

```bash
tok prove git log -50        # typically 85–92% on real repos
tok prove docker ps          # 60–75%
tok prove aws ec2 describe-instances  # 70–85% (JSON is extremely verbose)
tok prove go test ./...      # 40–80% (depends on failure rate)
```

Token estimates use cl100k_base average of ~3.5 chars/token, matching GPT-4 and Claude tokenizers closely for mixed code/prose output.

---

## How the hook works

After `tok init -g --copilot`, VS Code Copilot reads `~/.copilot/hooks/tok.json`:

```json
{
  "hooks": {
    "PreToolUse": [
      { "type": "command", "command": "/path/to/tok hook copilot", "timeout": 10 }
    ]
  }
}
```

Before every `run_in_terminal` call, Copilot pipes the tool input as JSON to `tok hook copilot` on stdin. tok rewrites the `command` field if a filter matches and returns it via stdout. Copilot then executes the rewritten command instead of the original.

```
agent: "tok git log -20"  ← rewritten transparently
            ↓
tok filter → compresses output → records savings
            ↓
agent sees 13 tokens instead of 1842
```

For Claude Code the same happens via `~/.claude/settings.json` `PreToolUse` hook targeting the `Bash` tool.

---

## The skill block

`tok init` injects this into your agent's instruction file:

```markdown
<!-- tok-skill: start — remove this block to disable -->
## Tok: Response Brevity
Be terse. Drop: articles, pleasantries, hedging, filler.
Never drop: not/never/no, numbers, technical terms, exact errors.
Fragments OK. Code, commits, PR bodies: normal prose.

## Tok: Cache Alignment
Place stable content (file contents, instructions) before volatile content
(tool results, current errors) so provider KV-cache prefixes stay stable.
Never restate what a tool result just showed — reference it directly.
<!-- tok-skill: end -->
```

The cache alignment hint is the part none of the other tools do well. By keeping stable content before volatile content, the provider's KV-cache can reuse prefixes across turns — this compounds across a long session and is invisible in per-request token counts but shows up in latency and cost.

---
