# RULES.md - Recast

## Enforced rules (AstraGraph policy: recast-default)

- Credential sanitization must remain enabled in generated outputs.
- Compiled tests must be deterministic and replay-safe.
- No external network calls are permitted during compile-time transforms.
- Input audit schema validation is mandatory before compilation.

## Human review required (PR, not direct commit)

- Any relaxation of sanitization behavior.
- Changes that may introduce nondeterministic generation.
- Changes to compile-time trust boundaries.

## Auto-blocked (AstraGraph fail-closed)

- Compilation of malformed/unsigned audit payloads.
- Generation paths that include unsanitized secrets.
- Compiler modes requiring online execution during test generation.
