# Task 017: Reorganize design-spec.md into Index + Section Files

**Type:** Task
**Status:** Complete
**Created:** 2026-02-13

## Context

`platform-docs/design-spec.md` is a 955-line living document that will grow as more services and features are added. Splitting it into a summary index with linked section files makes it easier for AI agents to find relevant information without loading the entire document. This was discussed and validated with AI chat (see Lesson 012).

## Changes

1. **Create `platform-docs/design-spec/` directory** — Extract each numbered section (1-15) into its own file (e.g., `01-related-adrs.md`, `02-dev-environment.md`, etc.)
2. **Rewrite `design-spec.md` as summary index** — Replace content with section headers, 1-2 sentence summaries, and links to section files. Keep Document History inline.
3. **Fix internal anchor links** — Update references that use `design-spec.md#section-anchor` to point to the new section files instead.

## Verification

- All links in the index file resolve to existing section files
- Grep for `design-spec.md#` confirms no remaining broken anchor links
- Content of all section files combined equals the original content (no information lost)
