## Summary

Describe what changed and why.

## Scope

- [ ] The change is focused and does not include unrelated work.
- [ ] User-visible behavior or documentation has been updated when needed.

## Validation

List the checks you ran, for example:

```text
go test ./...
go vet ./...
go build ./cmd/agent-config-inspector
go run ./cmd/agent-config-inspector verify .
```

## Safety and compatibility

- [ ] No private instruction text, secrets, or absolute workspace paths were added to fixtures or reports.
- [ ] Provider-specific behavior is backed by documented evidence or an explicit preview assumption.
- [ ] Backward compatibility or schema impact is called out when applicable.
