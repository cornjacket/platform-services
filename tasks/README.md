# Task Documents

This folder contains implementation task documents for platform-services features.

## Purpose

Task documents define:
- **What** the feature does (functionality)
- **Why** it's needed (context)
- **How** to implement it (design)
- **Acceptance criteria** (done when)

## Naming Convention

```
NNN-feature-name.md
```

- `NNN` — Sequential number (001, 002, ...)
- `feature-name` — Kebab-case description

## Template

```markdown
# Task NNN: Feature Name

**Status:** Draft | Ready | In Progress | Complete
**Created:** YYYY-MM-DD
**Updated:** YYYY-MM-DD

## Context

Why this feature is needed. Link to PROJECT.md phase if applicable.

## Functionality

What the feature does. User-facing or system behavior.

## Design

How to implement it:
- Components involved
- Data flow
- Key decisions

## Files to Create/Modify

- `path/to/file.go` — description

## Acceptance Criteria

- [ ] Criterion 1
- [ ] Criterion 2

## Notes

Open questions, alternatives considered, etc.
```

## Workflow

1. Create task doc with status **Draft**
2. Review and refine design
3. Change status to **Ready** when approved
4. Change status to **In Progress** when starting implementation
5. Change status to **Complete** when done and committed

## Index

| Task | Status | Description |
|------|--------|-------------|
| [001](001-outbox-processor.md) | Complete | Outbox Processor (NOTIFY/LISTEN) |
| [002](002-uuid-v7-migration.md) | Complete | UUID v7 Migration |
| [003](003-event-handler.md) | Complete | Event Handler (Redpanda consumer → projections) |
| [004](004-structured-logging.md) | Complete | Configurable Log Level and Format |
| [005](005-query-service.md) | Draft | Query Service |
