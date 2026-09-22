package testutil

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

type IntraHubNodeRecord struct {
	ID        int64
	SystemID  string
	HubID     int64
	Type      string
	RefHubID  int64
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ConnectMySQL opens and pings a MySQL database at the provided DSN.
func ConnectMySQL(dsn string) (*sql.DB, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open mysql connection: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping mysql: %w", err)
	}

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)
	return db, nil
}

// TruncateTables clears table rows across both databases for test isolation.
func TruncateTables(ctx context.Context, db *sql.DB) error {
	tables := []string{
		"sort_mistake.intra_hub_nodes",
		"sort_service.intra_hub_nodes",
	}

	for _, table := range tables {
		if _, err := db.ExecContext(ctx, fmt.Sprintf("TRUNCATE TABLE %s", table)); err != nil {
			return fmt.Errorf("failed to truncate %s: %w", table, err)
		}
	}
	return nil
}

// QueryIntraHubNode queries an intra_hub_node by ID from the specified database.
func QueryIntraHubNode(ctx context.Context, db *sql.DB, databaseName string, nodeID int64) (*IntraHubNodeRecord, error) {
	query := fmt.Sprintf("SELECT id, system_id, hub_id, type, ref_hub_id, name, created_at, updated_at FROM %s.intra_hub_nodes WHERE id = ?", databaseName)
	row := db.QueryRowContext(ctx, query, nodeID)

	var rec IntraHubNodeRecord
	err := row.Scan(&rec.ID, &rec.SystemID, &rec.HubID, &rec.Type, &rec.RefHubID, &rec.Name, &rec.CreatedAt, &rec.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("error querying %s.intra_hub_nodes id=%d: %w", databaseName, nodeID, err)
	}
	return &rec, nil
}
