## Summary

Describe the user-visible or engineering outcome.

## Rationale

Explain the problem and why this approach was chosen.

## Main changes

Describe the most important implementation changes.

## Validation

- [ ] `go test ./...`
- [ ] `go vet ./...`
- [ ] `go build ./cmd/nestworth`, when application or packaging behavior changed
- [ ] `gofmt -l cmd internal` is empty
- [ ] Manual application check with an isolated application-data environment, when UI or data behavior changed

## Screenshots or recordings

Include these for visible UI changes, or write “Not applicable.”

## Checklist

- [ ] The change is focused and does not include unrelated files.
- [ ] Tests and documentation are updated when needed.
- [ ] No credentials, private financial data, real database files, or machine-specific paths are included.
- [ ] Release-candidate checks were not run against the only copy of real user data.
- [ ] Financial values do not use binary floating point.
