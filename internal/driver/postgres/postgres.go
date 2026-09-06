package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/branchbase/branchbase/internal/driver"
)

// Config holds connection parameters for PostgreSQL
type Config struct {
	Host         string
	Port         int
	User         string
	Password     string
	BaseDatabase string
	SSLMode      string
}

// PostgresDriver manages database branching for PostgreSQL engines
type PostgresDriver struct {
	cfg Config
	db  *sql.DB
}

func init() {
	driver.Register("postgres", func(params map[string]interface{}) (driver.Driver, error) {
		cfg := Config{
			Host:         "127.0.0.1",
			Port:         5432,
			User:         "postgres",
			BaseDatabase: "myapp_dev",
			SSLMode:      "disable",
		}

		if h, ok := params["host"].(string); ok && h != "" {
			cfg.Host = h
		}
		if p, ok := params["port"].(int); ok && p > 0 {
			cfg.Port = p
		}
		if u, ok := params["user"].(string); ok && u != "" {
			cfg.User = u
		}
		if pwd, ok := params["password"].(string); ok {
			cfg.Password = pwd
		}
		if base, ok := params["base_database"].(string); ok && base != "" {
			cfg.BaseDatabase = base
		}

		return New(cfg)
	})
}

// New creates a new PostgresDriver instance
func New(cfg Config) (*PostgresDriver, error) {
	if cfg.SSLMode == "" {
		cfg.SSLMode = "disable"
	}
	return &PostgresDriver{cfg: cfg}, nil
}

func (d *PostgresDriver) Name() string {
	return "postgres"
}

func (d *PostgresDriver) Ping(ctx context.Context) error {
	if d.db == nil {
		return fmt.Errorf("postgres connection not initialized")
	}
	return d.db.PingContext(ctx)
}

func (d *PostgresDriver) BranchExists(ctx context.Context, branchName string) (bool, error) {
	dbName := d.formatDBName(branchName)
	query := "SELECT 1 FROM pg_database WHERE datname = $1"

	var exists int
	err := d.db.QueryRowContext(ctx, query, dbName).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// CreateBranch clones sourceBranch into targetBranch using PostgreSQL's TEMPLATE feature
func (d *PostgresDriver) CreateBranch(ctx context.Context, sourceBranch, targetBranch string) error {
	sourceDB := d.formatDBName(sourceBranch)
	targetDB := d.formatDBName(targetBranch)

	// Step 1: Terminate open connections to the source database
	terminateQuery := `
		SELECT pg_terminate_backend(pid)
		FROM pg_stat_activity
		WHERE datname = $1 AND pid <> pg_backend_pid();
	`
	_, _ = d.db.ExecContext(ctx, terminateQuery, sourceDB)

	// Step 2: Create new branch database from template
	createQuery := fmt.Sprintf("CREATE DATABASE %q TEMPLATE %q;", targetDB, sourceDB)
	_, err := d.db.ExecContext(ctx, createQuery)
	if err != nil {
		return fmt.Errorf("failed to create branch database %q from %q: %w", targetDB, sourceDB, err)
	}

	return nil
}

// DeleteBranch drops the specified branch database
func (d *PostgresDriver) DeleteBranch(ctx context.Context, branchName string) error {
	dbName := d.formatDBName(branchName)

	if dbName == d.cfg.BaseDatabase {
		return fmt.Errorf("cannot delete protected base database %q", dbName)
	}

	// Terminate active connections first
	terminateQuery := `
		SELECT pg_terminate_backend(pid)
		FROM pg_stat_activity
		WHERE datname = $1 AND pid <> pg_backend_pid();
	`
	_, _ = d.db.ExecContext(ctx, terminateQuery, dbName)

	dropQuery := fmt.Sprintf("DROP DATABASE IF EXISTS %q;", dbName)
	_, err := d.db.ExecContext(ctx, dropQuery)
	return err
}

// ListBranches returns all databases that start with the base_database prefix
func (d *PostgresDriver) ListBranches(ctx context.Context) ([]driver.BranchInfo, error) {
	prefix := d.cfg.BaseDatabase + "%"
	query := `
		SELECT datname, pg_database_size(datname)
		FROM pg_database
		WHERE datname LIKE $1
		ORDER BY datname ASC;
	`

	rows, err := d.db.QueryContext(ctx, query, prefix)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var branches []driver.BranchInfo
	for rows.Next() {
		var datname string
		var sizeBytes int64
		if err := rows.Scan(&datname, &sizeBytes); err != nil {
			return nil, err
		}

		branchName := strings.TrimPrefix(datname, d.cfg.BaseDatabase+"_")
		if datname == d.cfg.BaseDatabase {
			branchName = "main"
		}

		branches = append(branches, driver.BranchInfo{
			Name:        branchName,
			Database:    datname,
			CreatedAt:   time.Now(),
			SizeBytes:   sizeBytes,
			IsProtected: datname == d.cfg.BaseDatabase,
		})
	}

	return branches, nil
}

func (d *PostgresDriver) formatDBName(branch string) string {
	branch = strings.TrimSpace(branch)
	if branch == "" || branch == "main" || branch == "master" {
		return d.cfg.BaseDatabase
	}
	return fmt.Sprintf("%s_%s", d.cfg.BaseDatabase, branch)
}
