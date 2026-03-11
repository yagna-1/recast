<p align="center">
  <img src="./assets/logo/recast-cover.svg" alt="recast cover" width="78%" />
</p>

<p align="center">
  <strong>Turn any AI browser run into clean, production Playwright code.</strong>
</p>

<p align="center">
  <a href="https://go.dev/">
    <img src="https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go&logoColor=white" alt="Go version" />
  </a>
  <a href="./LICENSE">
    <img src="https://img.shields.io/badge/License-MIT-black" alt="MIT license" />
  </a>
  <a href="https://github.com/yagna-1/recast/actions/workflows/ci.yml">
    <img src="https://img.shields.io/github/actions/workflow/status/yagna-1/recast/ci.yml?label=CI" alt="CI status" />
  </a>
</p>

`recast` is a compiler that takes AI browser agent recordings and emits clean, readable, static Playwright test code - with no LLM required at replay time, no proprietary runtime dependencies, and no magic.

```
workflow-use JSON  ──┐
HAR file           ──┤  recast compile  ──▶  playwright.spec.ts
CDP event log      ──┤                  ──▶  .env.example
MCP tool call log  ──┘
```

## Why

AI browser agents (workflow-use, browser-use, Skyvern, Operator) re-reason through browser tasks at runtime — burning LLM tokens on every step, every run, forever. Recorded workflows are locked to proprietary runtimes and can't be committed to CI pipelines, reviewed by developers, or diff-ed.

`recast` fills the gap: compile once, run forever, on plain Playwright.

## Install

```bash
# Go install
go install github.com/yagna-1/recast/cmd/recast@latest

# Build from source
git clone https://github.com/yagna-1/recast
cd recast
go mod tidy
make build
./bin/recast version
```

## Quickstart

```bash
# Compile a workflow-use JSON recording to Playwright TypeScript
recast compile workflow.json

# Output:
# recast: INFO  detected format: workflow-use JSON
# recast: INFO  parsed 6 steps
# recast: INFO  [sanitize] 2 credential(s) replaced with environment variables
# recast: INFO  [emit] wrote playwright-ts to ./recast-out/test_login_and_download.spec.ts
# recast: INFO  [emit] wrote .env.example to ./recast-out/.env.example
# Exit code can be 1 when warnings are present (partial success).

# View output
cat ./recast-out/test_login_and_download.spec.ts
```

## Commands

### `recast compile`

Compile a workflow to Playwright test code.

```
recast compile [flags] <input-file>

Flags:
  -o, --output string       Output directory (default "./recast-out/")
  -t, --target string       playwright-ts | playwright-py | ir-json (default "playwright-ts")
      --no-optimize         Skip optimizer passes
      --no-harden           Skip selector hardening
      --inject-assertions   Add post-action assertions
      --replay-exact        Preserve recorded sequence; disable behavior-changing heuristic passes
      --strict              Exit 2 on any warning
  -v, --verbose             Detailed output
```

Examples:
```bash
recast compile workflow.json
recast compile workflow.json -t playwright-py -o ./tests/
recast compile trace.har --inject-assertions
recast compile mcp-log.jsonl -t ir-json
```

### `recast validate`

Validate an input file without compiling.

```bash
recast validate workflow.json
```

### `recast verify`

Verify compiled Playwright output against the original trace using screenshot-free signals (URL + key selectors).

```bash
recast verify ./recast-out/test_login_and_download.spec.ts --against workflow.json
# optional runtime check (no screenshot diff)
recast verify ./recast-out/test_login_and_download.spec.ts --against workflow.json --runtime
# strict runtime mode (fails if runtime cannot execute or test fails)
recast verify ./recast-out/test_login_and_download.spec.ts --against workflow.json --runtime --runtime-strict --runtime-timeout-sec 120
```

Notes:
- If the provided test file path is slightly off, `recast verify` auto-resolves to the closest `*.spec.ts`/`*.spec.py` file in the same directory and prints a warning.
- Runtime verification injects safe placeholder defaults for sanitized variables (`TEST_EMAIL`, `TEST_PASSWORD`, `RECAST_VAR_1..10`) when unset, so strict checks can run in clean environments.

### `recast formats`

List all supported input formats and output targets.

### `recast version`

Print version information.

### Exit codes

- `0` success
- `1` partial success with warnings
- `2` hard validation/compilation failure
- `3` input file not found/unreadable
- `4` unsupported input/output format

## What recast does

### 1. Ingestion

Parses input from supported formats into a normalized Intermediate Representation (IR). The IR is the AST — every optimization and code generation step operates on it.

**Supported input formats:**
- workflow-use JSON (`{"workflow_name": ..., "steps": [...]}`)
- HAR (HTTP Archive) — exported from any browser DevTools
- CDP Event Log — raw Chrome DevTools Protocol logs
- MCP Tool Call Log — from agents using MCP browser tools

### 2. Optimizer passes

| Pass | Name | Default |
|------|------|---------|
| 1 | Deduplication — remove consecutive identical actions | ✓ |
| 2 | Selector stabilization — upgrade fragile CSS to ARIA/role | ✓ |
| 3 | Wait inference — inject explicit waits | ✓ |
| 4 | **Credential sanitization** — replace secrets with env vars | ✓ always |
| 5 | Branch detection — flag missing conditional branches | ✓ |
| 6 | Assertion injection — add post-action assertions | opt-in |

### 3. Emit

Generates clean, readable test code in the target language.

**Supported targets:**
- `playwright-ts` — Playwright TypeScript (`.spec.ts`)
- `playwright-py` — Playwright Python (`.py`)
- `ir-json` — Normalized IR JSON (for debugging / community tools)

## Input → Output Example

**Input** (`workflow.json`):
```json
{
  "workflow_name": "login_and_download",
  "steps": [
    { "type": "navigate", "url": "https://app.example.com/login" },
    { "type": "fill", "selector": "#email", "value": "user@example.com" },
    { "type": "fill", "selector": "#password", "value": "hunter2" },
    { "type": "click", "selector": "button[type=submit]" },
    { "type": "wait_for", "selector": ".dashboard" }
  ]
}
```

**Output** (`test_login_and_download.spec.ts`):
```typescript
import { test, expect } from '@playwright/test';

test('login_and_download', async ({ page }) => {
  // Navigate to login page
  await page.goto('https://app.example.com/login');
  await page.waitForLoadState('networkidle');

  // the email input field
  await page.locator('#email').fill(process.env.TEST_EMAIL!);

  // the password input field
  await page.locator('#password').fill(process.env.TEST_PASSWORD!);

  // the Sign in button
  await page.locator('button[type=submit]').click();
  await page.waitForNavigation({ timeout: 10000 });

  await page.locator('.dashboard').waitFor({ state: 'visible', timeout: 30000 });
});
```

**`.env.example`**:
```bash
# Generated by recast — fill in actual values before running tests
# DO NOT commit this file with real values

# step_002: fill value was sanitized (detected: email pattern)
TEST_EMAIL=

# step_003: fill value was sanitized (detected: high-entropy string)
TEST_PASSWORD=
```

## Security

Credential sanitization runs **always** and **cannot be disabled**. Any `fill` step value that matches an email pattern, common credential field name, JWT token, credit card number, or high-entropy string is automatically replaced with an environment variable reference.

The `.env.example` file lists all sanitized variables. Fill in the real values locally; never commit them.

## Architecture

`recast` is structured identically to a programming language compiler:

```
Input Formats      →  Ingestion Adapters  →  IR (AST)  →  Optimizer Passes  →  Emitters  →  Output
```

Adding a new input format = write one ingestion adapter.
Adding a new output target = write one emitter.
Optimizer passes run on IR regardless of input or output.

The IR package (`recast-ir`) is published as a standalone Go module so agent frameworks can emit IR directly — bypassing file-based ingestion entirely.

## Development

```bash
git clone https://github.com/yagna-1/recast
cd recast
go mod tidy     # fetch cobra, viper, testify (populates go.sum)
make test       # all unit tests
make build      # compile binary
make golden-update  # generate golden files on first run
make golden-test    # run golden output tests (requires golden-update first)
```

## CI

GitHub Actions runs:
- `go mod tidy` consistency check
- `gofmt` format check
- `go vet`
- `make test`
- `make golden-test`
- `make build`

See `.github/workflows/ci.yml`.

## Contributing

See `CONTRIBUTING.md` for development workflow and PR expectations.

### Project structure

```
recast/
├── cmd/recast/          # CLI entrypoint
├── recast-ir/           # IR type definitions, builder, validator, marshaler
├── internal/
│   ├── ingestion/       # Format adapters (workflow-use, HAR, CDP, MCP)
│   ├── optimizer/       # Passes 1-6
│   ├── emitter/         # Playwright TS, Python emitters
│   └── config/          # Config management
├── pkg/recast/          # Public Go library API
└── testdata/            # Fixtures and golden files
```

## License

MIT
