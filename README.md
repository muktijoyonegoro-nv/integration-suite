# Integration Suite

A modular, polyglot integration testing harness designed to validate end-to-end multi-repository workflows (e.g., `sort-mistake`, `sort-service`) using Testcontainers and Podman.

---

## Prerequisites

* **Go**: 1.25+
* **Podman**: 5.x running rootless machine (`podman machine start`)
* **Protoc & Plugins** *(optional, only if regenerating protos)*: `protoc`, `protoc-gen-go`

---

## Quick Start

1. **Configure local environment**:
   Copy `.env.example` to `.env` and adjust the paths to your local repositories (e.g. `sort-mistake`, `sort-service`):
   ```bash
   cp .env.example .env
   ```

2. **Pre-pull public backing infrastructure images** (recommended for initial setup or slow connections):
   ```bash
   make pull-images
   ```

3. **Build local application images**:
   Build the container images directly from your local checkouts:
   ```bash
   # Build all local images:
   make build-image

   # Or build a specific service image:
   make build-image SERVICE=sort-mistake
   make build-image SERVICE=sort-service

   # (Optional shortcuts still supported):
   make build-sort-mistake
   make build-sort-service
   ```

4. **Run tests**:
   ```bash
   # Run all integration test suites across all repositories:
   make test

   # Run a specific repository's test scenarios:
   make test SUITE=sort-mistake

   # Run a specific scenario:
   make test SUITE=sort-mistake/intra_node

   # Filter specific test functions with custom timeout:
   make test SUITE=sort-mistake/intra_node RUN=TestSortTaskPipeline TIMEOUT=5m

   # (Optional shortcuts still supported):
   make test-intra-node
   make test-sort-mistake
   ```

5. **Clean up containers & dangling images**:
   ```bash
   # Remove managed test containers:
   make prune-containers

   # Remove dangling intermediate build images:
   make prune-images

   # Or clean everything (containers, images, test reports):
   make clean
   ```

---

## Configuration Architecture

The harness separates **public backing infrastructure** from **internal application services**:

```text
┌────────────────────────────────────────────────────────┐
│                      config.yaml                       │
│  - Public backing image versions (MySQL, Kafka, etc.)  │
│  - Podman binary path fallback                         │
└────────────────────────────────────────────────────────┘
                           ▲
                           │
┌────────────────────────────────────────────────────────┐
│                         .env                           │
│  - Local checkout paths (SORT_MISTAKE_DIR, etc.)       │
│  - Local image tags (SORT_MISTAKE_IMAGE, etc.)         │
└────────────────────────────────────────────────────────┘
```

### 1. Backing Infrastructure (`config.yaml`)
Public backing infrastructure image tags and runtime binary paths are centralized in `config.yaml`:

```yaml
podman:
  bin: /opt/podman/bin/podman
  images:
    mysql: "mysql:8.0"
    kafka: "confluentinc/confluent-local:7.6.0"
    redis: "redis:7-alpine"
    wiremock: "docker.io/wiremock/wiremock:3.5.2"
    flyway: "docker.io/flyway/flyway:11-alpine"
```

### 2. Local Application Services (`.env`)
Ninja Van application services (`sort-mistake`, `sort-service`) are **built locally from source** and never pulled from external registries. Each developer points to their local repository paths and custom image tags in `.env`:

```bash
# Local Repository Paths
SORT_MISTAKE_DIR=/Users/username/dev/sort-mistake
SORT_SERVICE_DIR=/Users/username/dev/sort-service

# Local Image Tags (optional, defaults to <service>:latest)
SORT_MISTAKE_IMAGE=sort-mistake:local
SORT_SERVICE_IMAGE=sort-service:local
```

### 3. Automatic Discovery & Environment Overrides
* The config loader (`pkg/config`) searches for `config.yaml` and `.env` starting from the current directory and traversing upwards to the project root.
* Custom file locations can be explicitly configured via `CONFIG_PATH` and `ENV_PATH`.
* Service names follow a standard kebab-to-snake case naming convention:
  * `sort-mistake` &rarr; `SORT_MISTAKE_DIR`, `SORT_MISTAKE_IMAGE`
  * `sort-service` &rarr; `SORT_SERVICE_DIR`, `SORT_SERVICE_IMAGE`

---

## Architecture & Layout

```text
integration-suite/
├── Makefile                         # Unified task runner & test triggers
├── config.yaml                      # Centralized image versions and Podman binary path
├── .env.example                     # Environment template for local repo paths & image tags
├── cmd/
│   └── pull-images/                 # CLI utility to pre-pull public images from config.yaml
├── docker/                          # Containerfiles for local service builds
│   ├── sort-mistake.Containerfile
│   └── sort-service.Containerfile
├── pkg/
│   ├── catalog/                     # Declarative service specs (SortMistake, SortService) & runner
│   │   ├── runner.go                # StartService launcher with dynamic port mapping
│   │   ├── service.go               # ServiceDefinition struct
│   │   ├── sort_mistake.go          # Default env vars, network aliases & health check for sort-mistake
│   │   └── sort_service.go          # Default env vars, network aliases & health check for sort-service
│   ├── config/                      # Upward-walking config.yaml & .env loader
│   ├── env/                         # Modular Builder, Podman socket discovery & safe pruning
│   │   ├── builder.go               # Fluent builder for MySQL, Kafka, Redis, WireMock
│   │   ├── environment.go           # Managed runtime environment & teardown
│   │   ├── podman.go                # Podman socket resolution
│   │   └── prune.go                 # Safe container pruning targeting harness labels
│   └── testutil/                    # Reusable DB connection/truncation, Kafka, Redis, and WireMock helpers
│       ├── db.go                    # MySQL connection, table truncation, query helpers
│       ├── kafka.go                 # Topic creation, proto reader/writer
│       ├── mock_aaa.go              # WireMock AAA token authentication stubs
│       └── redis.go                 # Redis flush and cache query helpers
├── proto/                           # Centralized dev-time Protobuf definitions
│   └── sortmistake/
│       ├── sort_node.proto          # Sourced .proto definitions
│       └── sort_node.pb.go          # Generated Go bindings (make proto-gen)
├── reports/                         # Generated test reports (e.g. junit.xml)
└── suites/                          # Per-repository, scenario-specific test suites
    ├── common/                      # Reusable scenario harness & connectivity assertions
    │   ├── config.go                # Declarative scenario configuration types
    │   ├── harness.go               # common.Setup(t, cfg), lifecycle & log dumping
    │   └── connectivity.go          # Automatic infrastructure & service connectivity checks
    └── sort-mistake/
        └── intra_node/              # Intra-Hub Node CUD lifecycle test scenario
            ├── config.go            # Declarative dependencies configuration
            └── sort_task_test.go    # End-to-end CRUD pipeline tests
```

---

## Key Design Principles

1. **Scenario-Level Granularity**: Each test scenario defines its own infrastructure via declarative `ScenarioConfig`—spinning up only the exact databases, caches, brokers, and mock services required for that specific scenario.
2. **Ultra-Lean Scenario Files**: Scenarios need only **2 files** (`config.go` + `<scenario>_test.go`). Boilerplate orchestration, connectivity checks, log dumps, and teardowns are handled by `suites/common`.
3. **Safe Pruning (Zero Image Deletions)**: All managed containers are stamped with `label=harness.managed=true`. Pruning targets *only* containers matching this label; images and external developer containers are never touched.
4. **No Host Port Collisions**: Dynamic ephemeral host ports are allocated at container startup. Tests resolve endpoints dynamically via harness helpers (`scenario.DB()`, `scenario.KafkaBroker()`, `scenario.Endpoint("service")`), allowing tests to run reliably in parallel without port conflicts.
5. **Dev-Time Protobufs**: Protobuf compilation is decoupled from test execution (`make proto-gen`). Tests run instantly without runtime tool dependencies on `protoc` or `protoc-gen-go`.
6. **Deterministic State Isolation**: Tests truncate databases and flush Redis caches before/after runs, avoiding state leakage between test executions.
7. **Automated Production Migrations (Flyway)**: Real migration scripts (`resources/db/migration`) are mounted directly from each microservice's checkout and applied via ephemeral Flyway containers, eliminating schema drift and keeping master reference data (regions, default hubs, settings) in sync with production.

---

## Test Execution Lifecycle: From Start to Finish

When you trigger an integration test (e.g., `make test-intra-node`), the harness executes a fully automated, multi-phase lifecycle—from container orchestration to post-test teardown and diagnostics:

```text
┌────────────────────────────────────────────────────────────────────────┐
│                        1. Pre-Flight Setup                             │
│  - Resolve Podman socket & configure DOCKER_HOST                       │
│  - Prune dangling containers from previous aborted runs                │
│  - Load config.yaml & .env overrides                                   │
└──────────────────────────────────┬─────────────────────────────────────┘
                                   ▼
┌────────────────────────────────────────────────────────────────────────┐
│            2. Backing Infrastructure Provisioning (env.Builder)        │
│  - Create isolated bridge network (e.g. sort-mistake-intra-node-net)   │
│  - Spin up MySQL: create databases & run Flyway migrations from repos  │
│  - Spin up Redis: boot instances with network aliases                  │
│  - Spin up Kafka: boot broker & pre-provision event topics             │
│  - Spin up WireMock: boot mock server & register AAA auth stubs        │
└──────────────────────────────────┬─────────────────────────────────────┘
                                   ▼
┌────────────────────────────────────────────────────────────────────────┐
│            3. Application Services Launch (catalog.StartService)       │
│  - Resolve image tags from .env (e.g. sort-mistake:local)              │
│  - Inject internal container DNS env vars (mysql:3306, kafka:9092, etc)│
│  - Boot application containers attached to scenario network            │
│  - Wait for readiness (listening port health check)                    │
│  - Resolve dynamic ephemeral host ports -> scenario.Endpoint(key)      │
└──────────────────────────────────┬─────────────────────────────────────┘
                                   ▼
┌────────────────────────────────────────────────────────────────────────┐
│            4. Automatic Connectivity Checks (Subtest)                  │
│  - Verify MySQL (SELECT 1 + table checks)                              │
│  - Verify Redis (PING -> PONG)                                         │
│  - Verify Kafka (test topic creation)                                  │
│  - Verify WireMock (/admin/health)                                     │
│  - Verify application HTTP endpoints                                   │
└──────────────────────────────────┬─────────────────────────────────────┘
                                   ▼
┌────────────────────────────────────────────────────────────────────────┐
│            5. Per-Test State Preparation & Pipeline Execution          │
│  - Truncate all database tables (scenario.TruncateTables)              │
│  - Flush Redis caches (scenario.FlushRedis)                            │
│  - Create isolated Kafka consumers (unique consumer group UUIDs)       │
│  - Execute test steps (HTTP API calls, Kafka events, DB assertions)    │
└──────────────────────────────────┬─────────────────────────────────────┘
                                   ▼
┌────────────────────────────────────────────────────────────────────────┐
│            6. Automatic Teardown & Failure Diagnostics (t.Cleanup)     │
│  - If test failed: automatically dump container logs (stdout/stderr)   │
│  - Terminate application containers                                    │
│  - Terminate backing containers (MySQL, Redis, Kafka, WireMock)        │
│  - Delete isolated bridge network                                      │
└────────────────────────────────────────────────────────────────────────┘
```

### Detailed Phase Breakdown

#### Phase 1: Pre-Flight & Runtime Discovery
1. **Podman Socket Resolution**: `Makefile` inspects the rootless Podman machine (`podman machine inspect`) to find the active Podman socket and sets `DOCKER_HOST=unix://<path>`.
2. **Startup Pruning**: The harness executes `PruneManagedContainers` to detect and remove any lingering containers labeled `harness.managed=true` from previous aborted runs, ensuring a clean slate.
3. **Configuration Loading**: `config.Load()` walks upwards from the scenario directory to discover and parse `config.yaml` and `.env`.

#### Phase 2: Backing Infrastructure Provisioning (`env.Builder`)
When the test calls `common.Setup(t, scenarioConfig)`, the harness initializes only the infrastructure declared in `scenarioConfig`:
* **Isolated Bridge Network**: Creates a dedicated Docker bridge network (e.g. `sort-mistake-intra-node-net`) so containers can communicate using container names as hostnames.
* **MySQL (`mysql:8.0`)**:
  * Starts container on the bridge network under the alias `mysql`.
  * Maps internal port `3306` to a random host port.
  * Connects via host port and executes `CREATE DATABASE IF NOT EXISTS` for all configured databases.
  * Executes DDL schema migrations from `pkg/testutil` (e.g., creating tables for `sort_mistake` and `sort_service`).
* **Redis (`redis:7-alpine`)**:
  * Starts separate container instances for each configured alias (e.g., `redis-sort-mistake`, `redis-sort`).
  * Maps internal port `6379` to random host ports and establishes Go `redis.Client` connections.
* **Kafka (`confluent-local:7.6.0`)**:
  * Starts Kafka broker on the bridge network under alias `kafka:9092`.
  * Resolves external host broker address.
  * Pre-creates all declared Kafka topics (e.g. `sort-mistake-nodes`).
* **WireMock (`wiremock:3.5.2`)**:
  * Starts WireMock on the bridge network under alias `mock-services:8080`.
  * Automatically registers default HTTP stubs, such as Ninja Van AAA OAuth token validation endpoints (`/__admin/mappings`).

#### Phase 3: Application Container Launch (`catalog.StartService`)
For each service defined in `scenarioConfig.Services`:
1. **Image Resolution**: Resolves the container image from `.env` (via `<SERVICE>_IMAGE`, e.g. `sort-mistake:local`) or defaults to `<service>:latest`.
2. **Environment Variable Injection**: Configures internal container networking environment variables so applications automatically connect to the backing containers within the bridge network:
   * `DB_HOST=mysql:3306`
   * `NV_KAFKA_SERVERS=kafka:9092`
   * `REDIS_HOST=redis-sort-mistake:6379`
   * `AAA_URL=http://mock-services:8080`
3. **Readiness Check**: The container is started with a readiness probe (e.g. `wait.ForListeningPort("9000/tcp").WithStartupTimeout(2 * time.Minute)`), waiting until the application is fully booted and accepting traffic before proceeding.
4. **Dynamic Port Mapping**: The internal service port is mapped to an ephemeral host port (e.g., `http://127.0.0.1:51234`), stored in the scenario and retrievable via `scenario.Endpoint("sort-mistake")`.

#### Phase 4: Automatic Connectivity Checks (`AssertConnectivity`)
Before executing any scenario test logic, `common.Setup` runs an automated subtest:
```go
t.Run("Connectivity Checks", func(t *testing.T) { ... })
```
This subtest verifies:
* **MySQL**: Executes `SELECT 1` ping and validates that all required tables exist.
* **Redis**: Sends `PING` and asserts `PONG` response across every configured Redis instance.
* **Kafka**: Verifies broker connectivity and publishes a test topic message.
* **WireMock**: Verifies `GET /__admin/health` returns `200 OK`.
* **Application Services**: Verifies mapped host endpoints are non-empty and accessible.

If any check fails, the test halts immediately with descriptive failure diagnostics rather than producing obscure downstream test failures.

#### Phase 5: Per-Test State Preparation & Pipeline Execution
Once setup completes, the scenario test file executes:
1. **Clean State Isolation**:
   ```go
   require.NoError(t, scenario.TruncateTables(ctx))
   require.NoError(t, scenario.FlushRedis(ctx))
   ```
   Wipes database rows and Redis keys to prevent state leakage between tests.
2. **Event Consumer Setup**:
   Creates test Kafka consumers with a unique consumer group (`uuid.NewString()`) to isolate message offset consumption.
3. **End-to-End Pipeline Execution**:
   * Sends real HTTP requests to the live application endpoint (`scenario.Endpoint("sort-mistake")`).
   * Asserts asynchronous events published to Kafka topics.
   * Asserts downstream database mutations using `scenario.DB()`.
   * Asserts cache invalidations or updates using `scenario.RedisClient(...)`.

#### Phase 6: Automatic Teardown & Failure Diagnostics (`t.Cleanup`)
The harness registers a cleanup callback via Go's standard `t.Cleanup()`:
1. **Failure Log Dumping**: If the test failed (`t.Failed()`), the harness automatically fetches and prints full `stdout` and `stderr` logs from application containers marked with `DumpLogsOnFailure: true`, making CI and local debugging effortless.
2. **Graceful Container Termination**: Stops and removes all application containers and backing containers.
3. **Network Cleanup**: Deletes the isolated Docker bridge network.

---

## Extending Test Scenarios

Every test scenario requires **only two files** in its directory:
1. `config.go` — Declarative infrastructure and service dependencies.
2. `<scenario>_test.go` — The integration test file calling `common.Setup(t, scenarioConfig)`.

### 1. Directory Structure Convention
Test scenarios are organized hierarchically under `suites/<repo-name>/<scenario-name>/`:

```text
suites/
└── <repo-name>/                     # e.g., sort-mistake, sort-service, hub-service
    └── <scenario-name>/             # e.g., intra_node, scan_result, parcel_routing
        ├── config.go                # Declarative dependencies only (~30 lines)
        └── <scenario>_test.go       # End-to-end test calling common.Setup (~80 lines)
```

### 2. Setting Up `config.go` (Declarative Dependencies)
Create a `config.go` in your scenario package defining a `common.ScenarioConfig`:

```go
package my_scenario

import (
	"integration-suite/pkg/catalog"
	"integration-suite/suites/common"
)

var scenarioConfig = common.ScenarioConfig{
	Name:        "sort-mistake/my_scenario",
	NetworkName: "sort-mistake-my-scenario-net",
	MySQL: common.MySQLConfig{
		Databases:   []string{"sort_mistake", "sort_service"},
		CheckTables: []string{"sort_mistake.my_table"},
	},
	Kafka: common.KafkaConfig{
		Topics: []string{"my-event-topic"},
	},
	RedisInstances: []string{"redis-sort-mistake"},
	WireMock:       true,
	Services: []common.ServiceConfig{
		{
			Def:         catalog.SortMistake,
			ExposePort:  "9000/tcp",
			EndpointKey: "sort-mistake",
		},
		{
			Def:               catalog.SortService,
			DumpLogsOnFailure: true,
		},
	},
}
```

### 3. Writing the Scenario Test (`<scenario>_test.go`)
Inside your test function, initialize the harness with a single call to `common.Setup(t, scenarioConfig)`:

```go
package my_scenario

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"integration-suite/suites/common"
)

func TestMyScenarioPipeline(t *testing.T) {
	ctx := context.Background()

	// 1. Boots containers, runs connectivity checks, and registers automatic teardown
	scenario := common.Setup(t, scenarioConfig)

	// 2. Clean per-test state
	require.NoError(t, scenario.TruncateTables(ctx), "tables must be clean")
	require.NoError(t, scenario.FlushRedis(ctx), "redis must be clean")

	apiURL := scenario.Endpoint("sort-mistake")
	redisClient := scenario.RedisClient("redis-sort-mistake")

	// 3. Run pipeline steps
	t.Run("Step 1 - Send Request", func(t *testing.T) {
		// ... call apiURL ...
		// ... assert Kafka event on scenario.KafkaBroker() ...
		// ... assert DB state on scenario.DB() ...
	})
}
```

Under the hood, `common.Setup(t, scenarioConfig)`:
- Spins up the requested backing containers (MySQL, Kafka, Redis, WireMock) and application containers.
- Automatically executes **Connectivity Checks** as an initial subtest (`t.Run("Connectivity Checks", ...)`).
- Registers a `t.Cleanup` callback that automatically dumps logs if the test fails (`DumpLogsOnFailure: true`) and tears down all containers and networks cleanly.

### 4. Registering a New Service in `pkg/catalog/`
If your test introduces a new application service:
1. Create `pkg/catalog/<service_name>.go` defining a `ServiceDefinition`:
   ```go
   package catalog

   import (
   	"time"
   	"github.com/testcontainers/testcontainers-go/wait"
   )

   var MyService = ServiceDefinition{
   	Name:         "my-service",
   	DefaultImage: "my-service:latest",
   	Aliases:      []string{"my-service"},
   	ExposedPorts: []string{"9000/tcp"},
   	Env: map[string]string{
   		"DB_HOST":          "mysql:3306",
   		"DB_NAME":          "my_database",
   		"NV_KAFKA_SERVERS": "kafka:9092",
   	},
   	WaitingFor: wait.ForListeningPort("9000/tcp").WithStartupTimeout(2 * time.Minute),
   }
   ```
2. Add a `docker/<service_name>.Containerfile` to build the service locally.
3. Add a build target in `Makefile` (`build-<service_name>`) calling `$(call build_service_image,...)`.
4. Add configuration entries to `.env.example`.

### 5. Writing Test Cases: Best Practices & Conventions

* **State Isolation per Test**:
  Always clean up database records and cache keys at the beginning of each test using harness helpers:
  ```go
  require.NoError(t, scenario.TruncateTables(ctx))
  require.NoError(t, scenario.FlushRedis(ctx))
  ```

* **Dynamic Host Ports**:
  Never hardcode `localhost:3306`, `localhost:9092`, or `localhost:9000`.
  - Use `scenario.DB()` for SQL queries.
  - Use `scenario.KafkaBroker()` for Kafka producer/consumer clients.
  - Use `scenario.RedisClient(alias)` for Redis.
  - Use `scenario.WireMockURL()` for mocking HTTP APIs.
  - Use `scenario.Endpoint("service-key")` for calling target application HTTP endpoints.

* **Asynchronous Assertions with `assert.Eventually`**:
  Distributed operations (e.g., event consumption over Kafka, cache invalidation, DB syncing) are asynchronous. Always assert outcomes using `assert.Eventually` with reasonable timeout and polling ticks:
  ```go
  assert.Eventually(t, func() bool {
  	record, err := testutil.QueryIntraHubNode(ctx, scenario.DB(), "sort_service", nodeID)
  	return err == nil && record != nil && record.Name == expectedName
  }, 10*time.Second, 200*time.Millisecond, "expected sort_service to sync node record")
  ```

* **Unique Kafka Consumer Groups**:
  When reading Kafka topics inside a test, create a uniquely named consumer group using `uuid.NewString()` (e.g. `testutil.NewKafkaReader(scenario.KafkaBroker(), topic, "test-"+uuid.NewString())`) to avoid offset pollution between tests.

* **Structured Subtests (`t.Run`)**:
  Group complex multi-step pipelines into clear subtests:
  ```go
  t.Run("Step 1 - Create Entity", func(t *testing.T) { ... })
  t.Run("Step 2 - Verify Event Published", func(t *testing.T) { ... })
  t.Run("Step 3 - Verify Downstream Sync", func(t *testing.T) { ... })
  ```

### 6. Running Scenarios Dynamically
You **do not** need to edit `Makefile` when adding a new test scenario! The harness supports dynamic arguments:

```bash
# Run any new repository or scenario immediately:
make test SUITE=<repo-name>/<scenario-name>

# Filter by test name within the scenario:
make test SUITE=<repo-name>/<scenario-name> RUN=TestMyPipeline TIMEOUT=5m
```

*(Optional)* If you want a quick shortcut for frequently executed scenarios, you can add an alias target in `Makefile`:
```makefile
test-<scenario-name>:
	@$(MAKE) test SUITE=<repo-name>/<scenario-name> TIMEOUT=5m
```

---

## Common Makefile Commands

| Command | Description |
|---|---|
| `make help` | Show list of available make targets and options |
| `make pull-images` | Pre-pull public backing infrastructure images defined in `config.yaml` |
| `make build-image` | Build local application container images (`SERVICE=all`, or `SERVICE=<service>`) |
| `make build-image SERVICE=<svc>` | Build a specific service container image (e.g. `SERVICE=sort-mistake`) |
| `make build-sort-mistake` | Shortcut to build only the `sort-mistake` container image |
| `make build-sort-service` | Shortcut to build only the `sort-service` container image |
| `make test` | Run all integration test suites (`SUITE=...`, `RUN=...`, `TIMEOUT=...`) |
| `make test SUITE=<path>` | Run a specific scenario or sub-package (e.g. `SUITE=sort-mistake/intra_node`) |
| `make test RUN=<pattern>` | Filter specific tests by regex (e.g. `RUN=TestSortTaskPipeline`) |
| `make test-sort-mistake` | Shortcut for `make test SUITE=sort-mistake TIMEOUT=10m` |
| `make test-intra-node` | Shortcut for `make test SUITE=sort-mistake/intra_node TIMEOUT=5m` |
| `make test-report` | Run test suites and generate JUnit XML report (`reports/junit.xml`) |
| `make prune-containers` | Safely stop and remove all managed test containers (`label=harness.managed=true`) |
| `make prune-images` | Safely prune dangling/intermediate build images |
| `make proto-gen` | Recompile `.proto` definitions into Go code |
| `make clean` | Prune test containers, remove dangling build images, and delete test reports |

---

## Troubleshooting

* **Podman socket connection errors (`DOCKER_HOST` not set or invalid)**:
  Ensure your Podman rootless machine is running:
  ```bash
  podman machine start
  ```
  Check that the socket exists with `podman machine inspect --format '{{.ConnectionInfo.PodmanSocket.Path}}'`.

* **Application build failures in `make build-image`**:
  Verify your `.env` contains valid absolute or relative paths to your local checkouts (`SORT_MISTAKE_DIR`, `SORT_SERVICE_DIR`).

* **Stuck or dangling test containers**:
  Run `make prune-containers` to remove any lingering containers labeled `harness.managed=true`. Your local developer containers and downloaded images will remain intact.

* **Image changes not reflected in tests**:
  Rebuild the image with `make build-image` (or `make build-<service>`) to ensure updated local source code is containerized.
