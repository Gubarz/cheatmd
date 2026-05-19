# Contributing to CheatMD

Thanks for your interest in contributing.

## Getting started

```bash
git clone https://github.com/Gubarz/cheatmd.git
cd cheatmd
go build ./cmd/cheatmd
go test ./...
```

Requires Go 1.26+.

## Making changes

1. Fork the repo and create a branch from `main`.
2. Make your changes.
3. Run `go test ./...` and `go vet ./...` locally.
4. Run `cheatmd --lint examples/` against the example cheats.
5. Open a pull request.

## Code style

- Run `gofmt` before committing.
- Keep functions focused and small.
- Add tests for new logic, especially in `pkg/parser` and `pkg/linter`.
- Preserve existing comments and docstrings unless they are wrong.

## Reporting bugs

Open an issue with:

- What you ran (command, flags, config).
- What you expected.
- What happened instead.
- OS and Go version.

## Feature requests

Open an issue describing the use case. If you have a cheat file that shows
the problem or the desired behavior, include it.

## Wiki

Documentation lives in the [Wiki](https://github.com/Gubarz/cheatmd/wiki).
If you spot an error or want to add a recipe, open a PR against the wiki
repo or mention it in an issue.

## License

By contributing you agree that your contributions will be licensed under the
MIT License.
