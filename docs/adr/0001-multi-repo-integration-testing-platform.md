# ADR 001: Multi-Repository Integration Test Platform Architecture

* **Status**: Accepted
* **Date**: 2026-09-23
* **Deciders**: Engineering Team
* **Technical Area**: Integration Testing, Container Runtime, Multi-Repo Microservices

---

## 1. Context and Problem Statement

The Ninja Van logistics platform consists of distributed microservices (e.g. `sort-mistake`, `sort-service`, hub services) that communicate asynchronously via Apache Kafka event streams, relational MySQL databases, Redis distributed caches, and HTTP/OAuth services.

Historically, validating cross-service workflows before deploying to staging environments presented significant operational and engineering challenges:

1. **High Risk of Cross-Boundary Regressions**: Unit tests rely heavily on mocks and cannot detect real contract discrepancies, Protobuf schema changes, message deserialization failures, or database migration drift across distinct repositories.
2. **Fragile Local Environments & Port Collisions**: Traditional local orchestration using static `docker-compose.yml` binds to fixed host ports (e.g., `3306`, `9092`, `6379`, `9000`), causing port collisions with developers' existing local services and preventing parallel test execution.
3. **Manual Lifecycle & Dangling Resources**: Developers frequently experienced orphaned containers, state contamination from previous test runs, and tedious manual setup/teardown steps.
4. **Delayed Defect Discovery**: Subtle integration bugs were only discovered downstream in shared staging or QA environments, increasing cycle times and blocking release pipelines.

We need a standardized, reproducible, and automated integration testing harness that enables developers and CI pipelines to validate multi-repository workflows against real backing infrastructure and containerized application binaries with zero environment drift.

---

## 2. Decision Drivers

* **Fidelity**: Tests must run against real production-grade infrastructure (MySQL, Kafka, Redis, WireMock), not in-memory substitutes.
* **Isolation & Determinism**: Each test scenario must be completely isolated, with dynamic ephemeral host ports, clean network boundaries, and automated state reset (truncating tables, flushing caches).
* **Zero Host Collisions**: Must allow developers and CI runners to execute tests concurrently without port conflicts.
* **Separation of Concerns**: Public backing infrastructure must be decoupled from developers' local checkout repositories and image tags.
* **Developer Ergonomics**: Adding a new test scenario should require minimal boilerplate, with automated connectivity validation and diagnostic log dumping on test failures.
* **Rootless & Security**: Must be compatible with rootless container engines (Podman) on developer workstations (macOS/Linux) and CI environments.

---

## 3. Considered Options

### Option 1: Docker Compose with Static Host Ports
* *Approach*: Maintain a central `docker-compose.yml` declaring all databases, brokers, and services on fixed ports (`3306`, `9092`, etc.).
* *Drawbacks*:
  * Frequent port collisions with local developer workstations.
  * Cannot run scenarios or suites in parallel.
  * Rigid orchestration; spinning up all containers regardless of whether a scenario needs them.
  * Teardown requires manual `docker compose down` scripts and fails to recover if a test process crashes.

### Option 2: Embedded / In-Memory Mocking (e.g., SQLite, In-Memory Kafka)
* *Approach*: Run tests in-process using mock or embedded equivalents of MySQL, Kafka, and Redis.
* *Drawbacks*:
  * Severe fidelity loss: dialect mismatches (MySQL vs SQLite), lack of real transaction semantics, and unverified Kafka broker configurations.
  * Does not test actual compiled service binaries or their Containerfile build processes.

### Option 3: Shared Staging / QA Environment Integration Testing
* *Approach*: Deploy PR artifacts to a shared staging environment and trigger end-to-end tests remotely.
* *Drawbacks*:
  * Slow feedback loop (minutes to hours).
  * Flaky test results caused by shared state mutations from concurrent teams.
  * Difficult to diagnose test failures remotely without local debugging access.

### Option 4: On-Demand Container Orchestration with Testcontainers and Podman (Selected)
* *Approach*: A centralized Go-based integration harness using `testcontainers-go` and rootless Podman to dynamically provision scenario-scoped containers with ephemeral host ports.

---

## 4. Decision Outcome

We chose **Option 4: Testcontainers with Rootless Podman**.

We have established a dedicated integration testing platform (`integration-suite`) built on the following foundational architecture:

### 4.1. Clean Configuration Separation
* **Public Infrastructure (`config.yaml`)**: Centralizes third-party container images (`mysql:8.0`, `confluentinc/confluent-local:7.6.0`, `redis:7-alpine`, `wiremock:3.5.2`, `flyway/flyway:11-alpine`) and runtime binary paths.
* **Local Repositories (`.env`)**: Maps service names to developers' local checkouts (`<SERVICE>_DIR`) and local image tags (`<SERVICE>_IMAGE`), allowing developers to test local code changes immediately without publishing to external registries.

### 4.2. Declarative Scenario-Level Granularity
Each scenario declares only its required dependencies via `ScenarioConfig`:
* **Isolated Bridge Network**: Created per scenario (`<name>-net`), enabling container-to-container communication using DNS aliases (`mysql:3306`, `kafka:9092`).
* **Dynamic Ephemeral Port Mapping**: Random host ports are allocated at boot time (`0.0.0.0:random`), eliminating port collisions.
* **Lean Scenario Layout**: New scenarios require only two files: `config.go` (declarative dependencies) and `<scenario>_test.go` (pipeline execution).

### 4.3. Automated Lifecycle & Diagnostics
* **Automatic Connectivity Checks**: `common.Setup(t, cfg)` executes a pre-test sanity check subtest verifying database connectivity, Kafka readiness, and HTTP endpoints before test logic begins.
* **Deterministic Isolation**: Helper methods (`TruncateTables`, `FlushRedis`) wipe database rows and cache keys before/after tests.
* **Diagnostic Log Dumping**: If a test fails, `t.Cleanup()` automatically retrieves and prints container logs (`stdout`/`stderr`) for flagged services.
* **Safe Container Pruning**: All managed containers are tagged with `label=harness.managed=true`. Pruning targets only managed test containers without touching developer containers or deleting downloaded images.

### 4.4. Dev-Time Protobuf Compilation
Protobuf definitions are committed and pre-compiled (`make proto-gen`). Test execution does not depend on runtime installations of `protoc` or `protoc-gen-go`.

### 4.5. Ephemeral Production Migration Execution (Flyway)
To prevent schema drift and eliminate duplicate hardcoded DDL scripts, the harness dynamically executes the actual Flyway migration scripts (`resources/db/migration`) directly from each service's local repository checkout using ephemeral `flyway/flyway` containers before application services boot. Master reference data (e.g. regions, countries, default configurations) is seeded identically to production.

---

## 5. Consequences and Tradeoffs

### Positive Consequences
* **High-Fidelity Contract Validation**: Real API calls, real Protobuf Kafka event streaming, and real MySQL/Redis synchronization are verified end-to-end.
* **Zero Port Conflicts & Parallel Execution**: Ephemeral ports allow running tests concurrently across multiple scenarios without port exhaustion or collisions.
* **Self-Healing & Clean Environments**: Dangling container pruning on startup and automatic `t.Cleanup()` ensure developer workstations stay clean.
* **Consistent CI and Local Experience**: Tests run identically on local macOS/Linux machines and in CI containers.

### Negative Consequences / Tradeoffs
* **Runtime Dependency**: Developers must have a container runtime installed and running (Rootless Podman machine).
* **Startup Latency**: Starting real containers (especially MySQL and Kafka) takes 10–30 seconds during scenario initialization.
* **Image Build Prerequisite**: Developers must build application container images locally (`make build-image`) when changing application code.

---

## 6. References & Related Documents

* [README.md](../../README.md) — Platform documentation and quick start guide.
* [config.yaml](../../config.yaml) — Centralized backing infrastructure versions.
* [pkg/env/builder.go](../../pkg/env/builder.go) — Test environment fluent builder.
* [suites/common/harness.go](../../suites/common/harness.go) — Scenario harness setup and teardown.
