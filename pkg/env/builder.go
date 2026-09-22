package env

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/kafka"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"

	"integration-suite/pkg/config"
	"integration-suite/pkg/testutil"
)

// DatabaseMigration defines a database target and the host directory containing Flyway .sql migrations.
type DatabaseMigration struct {
	Database     string
	MigrationDir string
}

type MySQLConfig struct {
	Databases  []string
	Migrations []DatabaseMigration
}

type KafkaConfig struct {
	Topics []string
}

// Builder constructs a tailored TestEnvironment for a specific test scenario.
type Builder struct {
	networkName    string
	mysqlConfig    *MySQLConfig
	kafkaConfig    *KafkaConfig
	redisAliases   []string
	enableWireMock bool
	cfg            *config.Config
}

func NewBuilder() *Builder {
	return &Builder{
		networkName:  "harness-net",
		redisAliases: make([]string, 0),
	}
}

func (b *Builder) WithConfig(cfg *config.Config) *Builder {
	b.cfg = cfg
	return b
}

func (b *Builder) WithNetwork(name string) *Builder {
	b.networkName = name
	return b
}

func (b *Builder) WithMySQL(cfg MySQLConfig) *Builder {
	b.mysqlConfig = &cfg
	return b
}

func (b *Builder) WithKafka(cfg KafkaConfig) *Builder {
	b.kafkaConfig = &cfg
	return b
}

func (b *Builder) WithRedisInstance(alias string) *Builder {
	b.redisAliases = append(b.redisAliases, alias)
	return b
}

func (b *Builder) WithWireMock() *Builder {
	b.enableWireMock = true
	return b
}

// Build provisions and starts only the requested components.
func (b *Builder) Build(ctx context.Context) (*Environment, error) {
	SetupPodmanEnvironment()

	// Startup cleanup: safely prune any dangling managed containers from previous runs
	_ = PruneManagedContainers(ctx)

	cfg := b.cfg
	if cfg == nil {
		var err error
		cfg, err = config.Load()
		if err != nil {
			return nil, fmt.Errorf("failed loading harness config: %w", err)
		}
	}

	env := &Environment{
		RedisContainers: make(map[string]testcontainers.Container),
		RedisAddrs:      make(map[string]string),
		RedisClients:    make(map[string]*redis.Client),
	}

	// 1. Bridge Network
	net, err := network.New(ctx, network.WithCheckDuplicate())
	if err != nil {
		return nil, fmt.Errorf("failed creating bridge network [%s]: %w", b.networkName, err)
	}
	env.Network = net

	// 2. MySQL (if requested)
	if b.mysqlConfig != nil {
		initialDB := "mysql"
		if len(b.mysqlConfig.Databases) > 0 {
			initialDB = b.mysqlConfig.Databases[0]
		}

		mysqlImage := cfg.GetImage("mysql", "mysql:8.0")
		mysqlC, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
			ContainerRequest: testcontainers.ContainerRequest{
				Image:        mysqlImage,
				ExposedPorts: []string{"3306/tcp"},
				Env: map[string]string{
					"MYSQL_ROOT_PASSWORD": "root",
					"MYSQL_DATABASE":      initialDB,
				},
				Labels:   DefaultLabels("mysql"),
				Networks: []string{net.Name},
				NetworkAliases: map[string][]string{
					net.Name: {"mysql"},
				},
				WaitingFor: wait.ForListeningPort("3306/tcp").WithStartupTimeout(2 * time.Minute),
			},
			Started: true,
		})
		if err != nil {
			_ = env.Teardown(ctx)
			return nil, fmt.Errorf("failed starting mysql container: %w", err)
		}
		env.MySQLContainer = mysqlC

		port, err := mysqlC.MappedPort(ctx, "3306/tcp")
		if err != nil {
			_ = env.Teardown(ctx)
			return nil, fmt.Errorf("failed resolving mysql host port: %w", err)
		}
		env.MySQLHostDSN = fmt.Sprintf("root:root@tcp(127.0.0.1:%s)/%s?multiStatements=true&parseTime=true", port.Port(), initialDB)

		db, err := testutil.ConnectMySQL(env.MySQLHostDSN)
		if err != nil {
			_ = env.Teardown(ctx)
			return nil, fmt.Errorf("failed connecting to mysql: %w", err)
		}
		env.DB = db

		// Ensure requested databases exist
		for _, dbName := range b.mysqlConfig.Databases {
			_, err := db.ExecContext(ctx, fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s`;", dbName))
			if err != nil {
				_ = env.Teardown(ctx)
				return nil, fmt.Errorf("failed creating database %s: %w", dbName, err)
			}
		}

		// Run Flyway migrations
		migrations := b.mysqlConfig.Migrations
		if len(migrations) == 0 {
			// Auto-discover migrations based on database names (e.g. sort_mistake -> service sort-mistake)
			for _, dbName := range b.mysqlConfig.Databases {
				serviceName := strings.ReplaceAll(dbName, "_", "-")
				repoDir := cfg.GetLocalRepoDir(serviceName, "")
				candidateDir := filepath.Join(repoDir, "resources", "db", "migration")
				if info, err := os.Stat(candidateDir); err == nil && info.IsDir() {
					migrations = append(migrations, DatabaseMigration{
						Database:     dbName,
						MigrationDir: candidateDir,
					})
				}
			}
		}

		for _, mig := range migrations {
			if mig.MigrationDir == "" {
				continue
			}
			if err := b.runFlywayMigration(ctx, net, "mysql", "3306", mig.Database, mig.MigrationDir); err != nil {
				_ = env.Teardown(ctx)
				return nil, fmt.Errorf("failed running flyway migration for [%s]: %w", mig.Database, err)
			}
		}
	}

	// 3. Redis Instances (if requested)
	redisImage := cfg.GetImage("redis", "redis:7-alpine")
	for _, alias := range b.redisAliases {
		redisC, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
			ContainerRequest: testcontainers.ContainerRequest{
				Image:        redisImage,
				ExposedPorts: []string{"6379/tcp"},
				Labels:       DefaultLabels(alias),
				Networks:     []string{net.Name},
				NetworkAliases: map[string][]string{
					net.Name: {alias},
				},
				WaitingFor: wait.ForListeningPort("6379/tcp").WithStartupTimeout(1 * time.Minute),
			},
			Started: true,
		})
		if err != nil {
			_ = env.Teardown(ctx)
			return nil, fmt.Errorf("failed starting redis [%s]: %w", alias, err)
		}
		env.RedisContainers[alias] = redisC

		rPort, err := redisC.MappedPort(ctx, "6379/tcp")
		if err != nil {
			_ = env.Teardown(ctx)
			return nil, fmt.Errorf("failed resolving redis port for [%s]: %w", alias, err)
		}
		addr := fmt.Sprintf("127.0.0.1:%s", rPort.Port())
		env.RedisAddrs[alias] = addr

		client, err := testutil.ConnectRedis(addr)
		if err != nil {
			_ = env.Teardown(ctx)
			return nil, fmt.Errorf("failed connecting to redis [%s]: %w", alias, err)
		}
		env.RedisClients[alias] = client
	}

	// 4. Kafka Broker (if requested)
	if b.kafkaConfig != nil {
		kafkaImage := cfg.GetImage("kafka", "confluentinc/confluent-local:7.6.0")
		kafkaC, err := kafka.Run(
			ctx,
			kafkaImage,
			network.WithNetwork([]string{"kafka"}, net),
			testcontainers.WithLabels(DefaultLabels("kafka")),
		)
		if err != nil {
			_ = env.Teardown(ctx)
			return nil, fmt.Errorf("failed starting kafka container: %w", err)
		}
		env.KafkaContainer = kafkaC

		brokers, err := kafkaC.Brokers(ctx)
		if err != nil || len(brokers) == 0 {
			_ = env.Teardown(ctx)
			return nil, fmt.Errorf("failed retrieving kafka broker address: %w", err)
		}
		env.KafkaBrokerAddr = brokers[0]

		// Provision requested topics
		for _, topic := range b.kafkaConfig.Topics {
			if err := testutil.EnsureTopic(env.KafkaBrokerAddr, topic, 1, 1); err != nil {
				_ = env.Teardown(ctx)
				return nil, fmt.Errorf("failed provisioning topic %s: %w", topic, err)
			}
		}
	}

	// 5. WireMock (if requested)
	if b.enableWireMock {
		wiremockImage := cfg.GetImage("wiremock", "docker.io/wiremock/wiremock:3.5.2")
		wiremockC, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
			ContainerRequest: testcontainers.ContainerRequest{
				Image:        wiremockImage,
				ExposedPorts: []string{"8080/tcp"},
				Labels:       DefaultLabels("wiremock"),
				Networks:     []string{net.Name},
				NetworkAliases: map[string][]string{
					net.Name: {"mock-services"},
				},
				WaitingFor: wait.ForListeningPort("8080/tcp").WithStartupTimeout(1 * time.Minute),
			},
			Started: true,
		})
		if err != nil {
			_ = env.Teardown(ctx)
			return nil, fmt.Errorf("failed starting wiremock container: %w", err)
		}
		env.WireMockContainer = wiremockC

		wPort, err := wiremockC.MappedPort(ctx, "8080/tcp")
		if err != nil {
			_ = env.Teardown(ctx)
			return nil, fmt.Errorf("failed resolving wiremock port: %w", err)
		}
		env.WireMockHostURL = fmt.Sprintf("http://127.0.0.1:%s", wPort.Port())

		if err := testutil.SetupWireMockAAA(ctx, env.WireMockHostURL); err != nil {
			_ = env.Teardown(ctx)
			return nil, fmt.Errorf("failed setting up wiremock AAA stubs: %w", err)
		}
	}

	return env, nil
}

// runFlywayMigration executes an ephemeral Flyway container to apply migrations to the specified database.
func (b *Builder) runFlywayMigration(ctx context.Context, net *testcontainers.DockerNetwork, dbHost, dbPort, dbName, migrationDir string) error {
	absDir, err := filepath.Abs(migrationDir)
	if err != nil {
		return fmt.Errorf("invalid migration directory path [%s]: %w", migrationDir, err)
	}
	if info, err := os.Stat(absDir); err != nil || !info.IsDir() {
		return fmt.Errorf("migration directory [%s] not found or is not a directory: %w", absDir, err)
	}

	flywayImage := b.cfg.GetImage("flyway", "docker.io/flyway/flyway:11-alpine")
	jdbcURL := fmt.Sprintf("jdbc:mysql://%s:%s/%s?allowPublicKeyRetrieval=true&useSSL=false", dbHost, dbPort, dbName)

	req := testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image: flywayImage,
			Cmd: []string{
				"-url=" + jdbcURL,
				"-user=root",
				"-password=root",
				"-connectRetries=60",
				"migrate",
			},
			Binds: []string{
				fmt.Sprintf("%s:/flyway/sql:ro", absDir),
			},
			Networks:   []string{net.Name},
			Labels:     DefaultLabels("flyway-" + dbName),
			WaitingFor: wait.ForExit().WithExitTimeout(2 * time.Minute),
		},
		Started: true,
	}

	c, err := testcontainers.GenericContainer(ctx, req)
	if err != nil {
		return fmt.Errorf("failed starting flyway container for db [%s]: %w", dbName, err)
	}
	defer func() {
		_ = c.Terminate(context.Background())
	}()

	state, err := c.State(ctx)
	if err != nil {
		return fmt.Errorf("failed inspecting flyway state for db [%s]: %w", dbName, err)
	}

	if state.ExitCode != 0 {
		var logMsg string
		if r, logErr := c.Logs(ctx); logErr == nil {
			if b, readErr := io.ReadAll(r); readErr == nil {
				logMsg = string(b)
			}
		}
		return fmt.Errorf("flyway migration failed for db [%s] with exit code %d:\n%s", dbName, state.ExitCode, logMsg)
	}

	return nil
}
