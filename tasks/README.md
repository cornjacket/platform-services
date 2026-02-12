# Task Documents

This folder contains all change records for platform-services. **Every change to the codebase must have an accompanying task document** — no exceptions.

## Two Types

| | Spec | Task |
|---|---|---|
| **Purpose** | New features, significant changes, design decisions | Bug fixes, minor changes, config tweaks, small improvements |
| **Design review** | Required (Draft → Ready before implementation) | Not required (record intent, then implement) |
| **Template** | Full (Context, Functionality, Design, Acceptance Criteria) | Lightweight (Context, Change, Verification) |

Both types share the same numbering sequence to preserve chronological ordering.

## Naming Convention

```
NNN-description.md
```

- `NNN` — Sequential number (001, 002, ...)
- `description` — Kebab-case description

**IMPORTANT: Source of Truth for Numbering**
The task list in this `README.md` is the **single source of truth** for task numbering. Before creating a new task document, always:
1.  Consult the `## Index` section below to find the highest sequential number.
2.  Assign the next available number (`Highest + 1`) to your new task.
3.  Add the new task to the `## Index` section of this `README.md` immediately upon creation.
Task file names *must* match the number and description in this index to maintain consistency and prevent conflicts.

## Spec Template (heavyweight)

```markdown
# Spec NNN: Feature Name

**Type:** Spec
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

### Spec Workflow

1. Create spec with status **Draft**
2. Review and refine design
3. Change status to **Ready** when approved
4. Change status to **In Progress** when starting implementation
5. Change status to **Complete** when done and committed

## Task Template (lightweight)

```markdown
# Task NNN: Description

**Type:** Task
**Status:** In Progress | Complete
**Created:** YYYY-MM-DD

## Context

Brief explanation of why this change is needed (1-3 sentences).

## Changes

What is being changed and how.

For grouped tasks, list each change as a numbered item:
1. **Change description** — what and why
2. **Change description** — what and why

## Verification

How to confirm the change(s) work correctly.
```

### Task Workflow

1. Create task with status **In Progress**
2. Implement the change
3. Change status to **Complete**
4. Commit task doc and code changes together

### Grouping Related Tasks

To avoid document proliferation, closely related tasks done in the same session can share a single task document. Each individual change is listed as a numbered item under **Changes**.

**Grouping criteria:**
- Changes must be **closely related** — same area of concern (e.g., all logging fixes, all config tweaks)
- Changes must be in the **same session** (same conversation/sitting)
- **Unrelated changes** in the same session still get separate task docs
- **Specs are never grouped** — each spec is a distinct design unit

## Index

| # | Type | Status | Description |
|---|------|--------|-------------|
| [001](001-outbox-processor.md) | Task | Complete | Outbox Processor |
| [002](002-uuid-v7-migration.md) | Task | Complete | UUID v7 Migration |
| [003](003-event-handler.md) | Task | Complete | Event Handler |
| [004](004-structured-logging.md) | Task | Complete | Configurable Log Level and Format |
| [005](005-query-service.md) | Task | Complete | Query Service |
| [006](006-automated-e2e-tests.md) | Task | Complete | Automated End-to-End Tests |
| [007](007-service-client-libraries.md) | Task | Complete | Service Client Libraries |
| [008](008-time-handling-strategy.md) | Task | Complete | Time Handling Strategy |
| [009](009-unit-tests.md) | Spec | Complete | Unit Tests |
| [010](010-integration-tests.md) | Spec | Complete | Integration Tests |
| [011](011-service-entry-points.md) | Spec | Complete | Service Entry Points |
| [012](012-component-tests.md) | Spec | Complete | Component Tests |
| [013](013-brainstorm-phase-2-implementation.md) | Task | In Progress | Brainstorm Phase 2 Implementation |
| [014](014-docker-compose-restructure.md) | Spec | Complete | Docker Compose Restructure & Platform Containerization |
| [015](015-embedded-migrations.md) | Spec | Complete | Embedded Migrations (Service-Owned, Auto-Applied on Startup) |
| [016](016-port-collision-shutdown.md) | Task | Complete | Bug: Port Collision Does Not Trigger Process Shutdown |
| [017](017-service-health-checks.md) | Task | In Progress | Service Health Check Endpoints |
