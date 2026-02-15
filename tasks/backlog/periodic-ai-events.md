# Periodic AI Events Workflow

## Status

Draft

## Problem

Currently, there is no standardized mechanism for defining and managing periodic tasks that AI agents should execute within the Cornjacket platform. This leads to ad-hoc approaches for recurring maintenance, monitoring, or generative AI activities, making it difficult to track execution, ensure consistency, and scale AI automation efforts.

## Proposal

Implement a "Periodic AI Events" workflow that allows for the definition, scheduling, and tracking of recurring tasks to be performed by AI agents. This workflow will be managed via a structured index file that provides a high-level overview and links to detailed task definitions.

## Design

### 1. Central Index File (`platform-docs/periodic-ai-events/README.md`)

A new `README.md` file will be created within a dedicated directory (e.g., `platform-docs/periodic-ai-events/`). This file will serve as the central index for all periodic AI tasks and will contain:

*   **Task Name/Description:** A concise title for each periodic task.
*   **Last Executed By AI:** A timestamp indicating when the AI last performed the task.
*   **Periodicity:** How often the AI should execute the task (e.g., Daily, Weekly, Monthly, Quarterly).
*   **Link to Task Details:** A Markdown link to a separate document (child file) providing a detailed description of the task, its objectives, required inputs/outputs, and execution instructions for the AI.

### 2. Detailed Task Documents (Child Files)

Each entry in the central index file will link to a dedicated Markdown file (e.g., `platform-docs/periodic-ai-events/task-name.md`) that outlines:

*   **Task Objective:** What the AI should achieve.
*   **Execution Steps:** A clear, step-by-step guide for the AI to follow.
*   **Context/Dependencies:** Any specific project knowledge, tools, or data required.
*   **Verification Criteria:** How the AI (or a human reviewer) can confirm successful completion.
*   **Error Handling:** Instructions on what to do if the task encounters issues.

### 3. Integration with AI Agent Workflow

AI agents will periodically consult the central index file (`platform-docs/periodic-ai-events/README.md`) to identify tasks that are due for execution based on their `Last Executed By AI` timestamp and `Periodicity`. Upon executing a task, the AI agent will be responsible for updating the `Last Executed By AI` timestamp in the index file.

## Benefits

*   **Standardization:** Provides a consistent framework for defining and managing recurring AI tasks.
*   **Transparency:** Clearly shows what periodic tasks the AI is responsible for and their execution status.
*   **Scalability:** Easily add new periodic tasks without modifying core AI logic.
*   **AI Readability:** Leverages the index-document pattern (as discussed in AI Builder Lesson 013) to allow AI agents to efficiently discover and execute relevant tasks.

## Open Questions / Future Considerations

*   Mechanism for AI agents to automatically update the `Last Executed By AI` timestamp (e.g., a specific tool call).
*   Handling of task dependencies or execution order.
*   Notification mechanisms for failed periodic tasks.
