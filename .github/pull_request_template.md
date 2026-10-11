## What and why

## How it was checked

- [ ] `gofmt`, `go vet`, `go test` and `-race` on the touched packages
- [ ] `golangci-lint run --new-from-rev=origin/dev` is clean
- [ ] `npm run check` and `npm test` (if the frontend changed)
- [ ] User-facing strings added in both languages
- [ ] Commits are signed off (`git commit -s`)
