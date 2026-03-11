# Contributing to recast

Thanks for helping improve `recast`.

## Development setup

Requirements:
- Go 1.22+

Setup:

```bash
git clone https://github.com/yagna-1/recast
cd recast
go mod tidy
```

## Local quality checks

Run these before opening a PR:

```bash
make test
make golden-test
make build
make vet
```

If you intentionally change emitted output:

```bash
make golden-update
make golden-test
```

## Pull request expectations

- Keep changes scoped and focused.
- Add tests for behavior changes.
- Keep generated output deterministic.
- Do not commit real credentials in fixtures or docs.
- Ensure `go.mod` and `go.sum` are tidy when dependencies change.

## Coding guidelines

- Use `gofmt` formatting.
- Return contextual errors (`fmt.Errorf("module: context: %w", err)`).
- Avoid panics in library code.
- Keep imports at the top of files.

## Security notes

`recast` treats all input traces as untrusted. Credential sanitization is mandatory and should remain non-bypassable.
