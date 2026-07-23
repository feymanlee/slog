# Issue tracker: GitHub

Issues and PRDs for this repo live as GitHub issues. Use the `gh` CLI for all operations.

## Conventions

- Create: `gh issue create --title "..." --body "..."`
- Read: `gh issue view <number> --comments`
- List: `gh issue list --state open --json number,title,body,labels,comments`
- Comment: `gh issue comment <number> --body "..."`
- Label: `gh issue edit <number> --add-label "..."` or `--remove-label "..."`
- Close: `gh issue close <number> --comment "..."`

Infer the repository from `git remote -v`; `gh` does this automatically inside the clone.

## Pull requests as a triage surface

**PRs as a request surface: no.**

GitHub shares one number space across issues and pull requests. Resolve an
ambiguous `#42` with `gh pr view 42`, falling back to `gh issue view 42`.

## Skill operations

When a skill says "publish to the issue tracker", create a GitHub issue.

When a skill says "fetch the relevant ticket", run:

`gh issue view <number> --comments`

## Wayfinding operations

The map is one issue labelled `wayfinder:map`; child issues are tickets.

- Create maps with `gh issue create --label wayfinder:map`.
- Link children using GitHub sub-issues when available.
- Label children with `wayfinder:<type>`.
- Represent blockers using GitHub issue dependencies when available.
- Claim work with `gh issue edit <number> --add-assignee @me`.
- Resolve by commenting with the answer, closing the issue, and updating the map.
