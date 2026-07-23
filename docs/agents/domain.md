# Domain Docs

This repository uses a single-context domain documentation layout.

## Before exploring

Read these files when they exist:

- `CONTEXT.md` at the repository root.
- Relevant ADRs under `docs/adr/`.

If they do not exist, proceed silently. Domain-modeling skills create them
lazily when terminology or architectural decisions are resolved.

## File structure

```
/
|-- CONTEXT.md
|-- docs/adr/
|-- dlp/
|-- internal/
`-- modules/
```

## Vocabulary

Use terms defined in `CONTEXT.md` consistently in issues, proposals, tests,
and code. Treat missing terminology as a possible domain-modeling gap.

## ADR conflicts

Explicitly identify output that contradicts an existing ADR instead of
silently overriding the recorded decision.
