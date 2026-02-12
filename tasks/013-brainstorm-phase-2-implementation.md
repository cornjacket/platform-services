# Task 013: Brainstorm Phase 2 Implementation

**Type:** Task
**Status:** In Progress
**Created:** 2026-02-11

## Context

Phase 1: Local Skeleton has been completed, providing a foundational end-to-end event flow. The project is now transitioning to Phase 2: Local Full Stack. This task initiates a brainstorming session to detail the implementation plan for Phase 2, including identifying specific technologies, architectural considerations, and potential design ramifications.

## Changes

-   Review the existing goals for Phase 2 as defined in `platform-docs/PROJECT.md`.
-   Brainstorm specific implementation steps for each goal.
-   Identify potential architectural challenges or decisions required.
-   Consider the impact on the current system design.
-   Document initial thoughts and decisions, potentially leading to further tasks or spec documents.
-   Ensure that containerization for the platform is also a flow in the Makefile.
-   In order to run the `cornjacket-platform` as a component inside docker-compose (i.e. simulating the side-car pattern), we need to containerize the `cornjacket-platform/platform-services/cmd/main.go`.
-   **Brainstorming Questions:**
    -   Should `docker-compose-*.yaml` files be moved to a separate folder (e.g., `platform-services/docker-compose/`) to reduce clutter in the root directory? What are the pros and cons?

        **Answer:** Yes, moving `docker-compose-*.yaml` files to a dedicated subdirectory like `platform-services/docker-compose/` is generally a good practice to reduce clutter in the root of `platform-services/`.

        *   **Pros:**
            *   **Reduced Root Clutter:** Keeps the `platform-services/` root cleaner, making it easier to navigate and locate primary project files (e.g., `go.mod`, `Makefile`, `cmd/`).
            *   **Improved Organization:** Centralizes all Docker Compose related files, making them easier to find and manage.
            *   **Clearer Intent:** Signals that these are configuration files for container orchestration rather than primary code artifacts.
            *   **Scalability:** As the number of Docker Compose configurations grows, a dedicated folder prevents the root directory from becoming overwhelming.

        *   **Cons:**
            *   **Slightly Longer Paths in Commands:** Commands using `docker compose -f` will require the subdirectory path (e.g., `docker compose -f docker-compose/docker-compose.yaml ...`). This is a minor increase in verbosity but easily managed with `make` targets.
            *   **Initial Setup/Migration:** Requires an initial move and update of any existing scripts/documentation that reference the old paths.

        **Recommendation:** The pros generally outweigh the cons. A dedicated `platform-services/docker-compose/` directory (or similar, perhaps `platform-services/deploy/docker-compose/`) is advisable. Your `Makefile` targets would then need to be updated to include these paths.

    -   Should we containerize the `cornjacket-platform` binary at this step? Or leave it as a binary? Or have multiple configs for each? The more options we have the more confusing it can become. Is there any reason to have a binary of the platform at this stage of the testing? Is it really required or can we use the skeleton `docker-compose` only to test the platform binary?

        **Answer:** This is a crucial design decision that impacts development speed, testing complexity, and the fidelity of your local environment.

        *   **Option A: Always run as a binary (as currently done with the skeleton system).**
            *   **Pros:**
                *   **Fastest Iteration for Go Code:** For Go developers, compiling and running the binary directly is often the fastest way to see code changes reflected. No Docker image build times.
                *   **Easier Debugging (Host Tools):** Can use host-based debuggers and profilers directly on the Go process without container complexities.
                *   **Matches "Skeleton" Philosophy:** Aligns with the idea of a minimal, highly optimized local dev environment.
            *   **Cons:**
                *   **Less Production Fidelity:** The local environment deviates from the production deployment model where the application *will* run in a container. This can mask container-specific issues (e.g., networking, resource limits, file system access inside a container).
                *   **Increased Configuration Management:** Your `docker-compose` files need to manage external network access to the binary running on the host.

        *   **Option B: Always run `cornjacket-platform` as a container.**
            *   **Pros:**
                *   **High Production Fidelity:** Local environment closely mirrors production, reducing "it works on my machine" issues. Container-specific issues are caught earlier.
                *   **Simplified Docker Compose Networking:** All services are within the Docker network, simplifying inter-service communication configuration.
                *   **Consistent CI/CD:** Local build process (building the container image) is more similar to CI/CD.
            *   **Cons:**
                *   **Slower Iteration for Go Code:** Requires rebuilding the Docker image for every Go code change, which adds overhead. This can be mitigated with techniques like bind mounts for Go source code or multi-stage builds, but it's generally still slower than running a binary.
                *   **More Complex Debugging:** Debugging inside a container can be involved.

        *   **Option C: Multiple Configurations (Binary for Skeleton Dev, Container for Full Stack/Production Fidelity).**
            *   **Pros:**
                *   **Best of Both Worlds:** Combines rapid iteration for core Go development with high production fidelity for full-stack integration and final testing.
                *   **Optimized for Different Phases/Needs:** Skeleton system is optimized for speed; full stack is optimized for realism.
            *   **Cons:**
                *   **Increased Complexity in `docker-compose` Management:** Requires carefully crafted `docker-compose` files to switch between binary and container modes for the monolith. This is achievable but adds more configuration overhead.
                *   **Cognitive Overhead:** Developers need to understand when to use which configuration.

        **Is there any reason to have a binary of the platform at this stage of the testing? Is it really required or can we use the skeleton `docker-compose` only to test the platform binary?**
        *   **Reason for Binary (Current Skeleton):** The primary reason is **development speed and ease of debugging** for Go code. It allows developers to quickly compile and run their application locally, attach debuggers easily, and bypass Docker image build times for every small code change. This is a very valid reason for a rapid iteration development environment.
        *   **Testing the Platform Binary:** Yes, the skeleton `docker-compose` *only* to test the platform binary is perfectly fine for unit, integration, and even e2e tests that focus on the *functionality* of the platform binary itself, interacting with its dependencies (Redpanda, Postgres). The tests verify business logic, data flow, etc.

        **Recommendation:**
        Option C (Multiple Configurations) seems to align best with your stated goals of both fast design iteration and moving towards a "Local Full Stack" mirroring production.
        *   **Keep the existing "skeleton" setup (platform binary + core services)** for rapid Go development and early e2e testing.
        *   **For the "full stack" setup, *transition to running the `cornjacket-platform` as a container***. This configuration would be used for integration testing with Traefik, EMQX and other containerized services, providing high production fidelity.
        This requires careful design of your `docker-compose` files to make the switch clean, likely using override files and/or profiles as discussed previously. This would also mean that your `platform-services/Makefile` targets would need to reflect this choice (e.g., `make skeleton-up` runs the binary, `make fullstack-up` runs the container).

    -   How should we manage multiple `docker-compose` configurations?

        **Answer:** The most effective way to manage multiple `docker-compose` configurations in your scenario is by leveraging Docker Compose's ability to **combine multiple Compose files** and use **profiles**. This allows for highly flexible and modular setups without duplicating configuration.

        **1. How Different Files Leverage Each Other (Combining Compose Files)**

        Docker Compose allows you to specify multiple `-f` flags, where subsequent files can *add* to or *override* definitions from previous files.

        *   **Order Matters:** When you use `docker compose -f file1.yaml -f file2.yaml`, `file2.yaml` can override settings for services defined in `file1.yaml` or add entirely new services.
        *   **Extension:** This is a powerful feature for building layers of configuration.

        Let's refine the file strategy:

        *   **`docker-compose.yaml` (Base/Common Services):**
            *   This file defines the *absolute minimum* common services that are always part of *any* local development environment.
            *   **Example:** `redpanda`, `postgres`. These are likely always needed for both skeleton and full-stack.
            *   This file **should not use `profiles`** for its services. They are always active when this file is included.
            *   **Location:** `platform-services/docker-compose.yaml`

        *   **`docker-compose.fullstack.yaml` (Adds Full Stack Components):**
            *   This file adds the *additional* services that differentiate the "full stack" environment from the base.
            *   **Example:** `traefik`, `emqx`, `ai-inference`.
            *   Services in this file **should use `profiles: ['fullstack']`**. This ensures they only start when the `fullstack` profile is activated.
            *   This file can also *extend* or *override* services defined in `docker-compose.yaml` (e.g., if `redpanda` needs a different config in fullstack).
            *   **Location:** `platform-services/docker-compose.fullstack.yaml`

        *   **`docker-compose.monolith.yaml` (Overrides for Monolith Binary):**
            *   This file (or perhaps just `docker-compose.yaml` itself could contain this if it's the *only* way the monolith runs in dev) defines how your `platform` monolith binary service runs.
            *   Crucially, this file should *override* the default `platform` service (which might be a basic container) to run your Go binary, possibly mounting your source code for hot-reloading if desired.
            *   **Location:** `platform-services/docker-compose.monolith.yaml` (or incorporate directly into `docker-compose.yaml`)

        *   **`docker-compose.e2e.yaml` (E2E Test Overrides):**
            *   This file provides specific overrides for running end-to-end tests.
            *   **Example:** It might change the `command` for the `platform` service to include specific test flags, mount test data, or even replace services with test stubs.
            *   It generally should *not* define new services, but modify existing ones for testing purposes.
            *   **Location:** `platform-services/docker-compose.e2e.yaml`

        **2. Using `make` for Starting and Stopping Docker Compose**

        Yes, using `make` is an excellent way to encapsulate these complex `docker compose` commands, making them simpler and more consistent to execute.

        You would define targets in your `Makefile` (located in `platform-services/Makefile`) like so:

        ```makefile
        # Define common compose files
        COMPOSE_BASE_FILES = docker-compose.yaml

        # Skeleton setup (base services + monolith binary)
        COMPOSE_SKELETON_FILES = $(COMPOSE_BASE_FILES) docker-compose.monolith.yaml

        # Full stack setup (base + fullstack additions + monolith binary)
        COMPOSE_FULLSTACK_FILES = $(COMPOSE_BASE_FILES) docker-compose.monolith.yaml docker-compose.fullstack.yaml

        .PHONY: skeleton-up skeleton-down fullstack-up fullstack-down

        skeleton-up:
            @echo "Starting skeleton subsystem..."
            docker compose -f $(COMPOSE_SKELETON_FILES) up -d --build

        skeleton-down:
            @echo "Stopping skeleton subsystem..."
            docker compose -f $(COMPOSE_SKELETON_FILES) down

        fullstack-up:
            @echo "Starting full stack local environment..."
            docker compose -f $(COMPOSE_FULLSTACK_FILES) up -d --build --profile fullstack

        fullstack-down:
            @echo "Stopping full stack local environment..."
            docker compose -f $(COMPOSE_FULLSTACK_FILES) down

        # Target for running e2e tests against the skeleton
        e2e-skeleton:
            @echo "Running e2e tests against skeleton..."
            docker compose -f $(COMPOSE_SKELETON_FILES) -f docker-compose.e2e.yaml up --build --exit-code-from e2e-runner && docker compose -f $(COMPOSE_SKELETON_FILES) -f docker-compose.e2e.yaml down
            # Assuming 'e2e-runner' is a service defined in docker-compose.e2e.yaml that runs your tests
        ```

        **3. Execution Location:**

        *   **`docker compose` commands:** These commands should always be run from the **`platform-services/` root directory**, where your `docker-compose*.yaml` files are located. This is standard practice and ensures Docker Compose can find the necessary files.
        *   **`make` commands:** Your `Makefile` should also reside in the **`platform-services/` root directory**. Therefore, you would run `make skeleton-up`, `make fullstack-up`, etc., directly from `platform-services/`.

        **Example Usage Flow:**

        1.  **For core feature development (using skeleton):**
            ```bash
            cd platform-services
            make skeleton-up
            # ... develop features ...
            make skeleton-down
            ```

        2.  **For full-stack local environment (Phase 2):**
            ```bash
            cd platform-services
            make fullstack-up
            # ... test full system ...
            make fullstack-down
            ```

        3.  **For running e2e tests:**
            ```bash
            cd platform-services
            make e2e-skeleton # or make e2e-fullstack if you create that target
            ```

        This modular approach provides clarity, reduces redundancy, and gives you fine-grained control over your local environments while simplifying common operations via `make`.

## Verification

-   Initial brainstorming notes are captured (e.g., in this task document or linked sub-documents).
-   A clear understanding of the next steps for Phase 2 implementation is established.
-   [ ] Update PROJECT.md (or other relevant high-level progress tracking document) to reflect task completion, if applicable.
