package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"
	"time"

	"errors"
	"github.com/branchbase/branchbase/internal/git"
	"github.com/lib/pq"

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

// PoolConfig controls the database connection pool used by a PostgresDriver.
type PoolConfig struct {
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

var defaultPoolConfig = PoolConfig{
	MaxOpenConns:    25,
	MaxIdleConns:    5,
	ConnMaxLifetime: 5 * time.Minute,
}

// DSN returns the PostgreSQL connection URL string for the pgx driver
func (c Config) DSN() string {
	host := c.Host
	if host == "" {
		host = "127.0.0.1"
	}
	port := c.Port
	if port <= 0 {
		port = 5432
	}
	dbName := c.BaseDatabase
	if dbName == "" {
		dbName = "postgres"
	}
	sslMode := c.SSLMode
	if sslMode == "" {
		sslMode = "disable"
	}

	u := &url.URL{
		Scheme: "postgres",
		Host:   fmt.Sprintf("%s:%d", host, port),
		Path:   dbName,
	}
	if c.User != "" {
		if c.Password != "" {
			u.User = url.UserPassword(c.User, c.Password)
		} else {
			u.User = url.User(c.User)
		}
	}
	q := u.Query()
	q.Set("sslmode", sslMode)
	u.RawQuery = q.Encode()

	return u.String()
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
		if ssl, ok := params["sslmode"].(string); ok && ssl != "" {
			cfg.SSLMode = ssl
		}

		return NewWithPoolConfig(cfg, poolConfigFromParams(params))
	})
}

func poolConfigFromParams(params map[string]interface{}) PoolConfig {
	poolConfig := defaultPoolConfig

	if maxOpenConns, ok := params["max_open_conns"].(int); ok && maxOpenConns > 0 {
		poolConfig.MaxOpenConns = maxOpenConns
	}
	if maxIdleConns, ok := params["max_idle_conns"].(int); ok && maxIdleConns >= 0 {
		poolConfig.MaxIdleConns = maxIdleConns
	}

	return poolConfig
}

// New creates a new PostgresDriver instance and initializes the connection pool
func New(cfg Config) (*PostgresDriver, error) {
	return NewWithPoolConfig(cfg, defaultPoolConfig)
}

// NewWithPoolConfig creates a PostgresDriver with the supplied connection pool settings.
func NewWithPoolConfig(cfg Config, poolConfig PoolConfig) (*PostgresDriver, error) {
	if cfg.SSLMode == "" {
		cfg.SSLMode = "disable"
	}

	db, err := sql.Open("postgres", cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("failed to open postgres connection: %w", err)
	}

	db.SetMaxOpenConns(poolConfig.MaxOpenConns)
	db.SetMaxIdleConns(poolConfig.MaxIdleConns)
	db.SetConnMaxLifetime(poolConfig.ConnMaxLifetime)

	return &PostgresDriver{
		cfg: cfg,
		db:  db,
	}, nil
}

// NewWithDB creates a PostgresDriver with an existing database handle (useful for unit tests and custom pools)
func NewWithDB(cfg Config, db *sql.DB) *PostgresDriver {
	if cfg.SSLMode == "" {
		cfg.SSLMode = "disable"
	}
	return &PostgresDriver{
		cfg: cfg,
		db:  db,
	}
}

// DB returns the underlying *sql.DB connection pool
func (d *PostgresDriver) DB() *sql.DB {
	return d.db
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
	if d.db == nil {
		return false, fmt.Errorf("postgres connection not initialized")
	}
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
	if d.db == nil {
		return fmt.Errorf("postgres connection not initialized")
	}
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
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "42P04" {
			// SQLSTATE 42P04 = duplicate_database, treat as idempotent success
			return nil
		}
		return fmt.Errorf("failed to create branch database %q from %q: %w", targetDB, sourceDB, err)
	}

	return nil
}

// DeleteBranch drops the specified branch database
func (d *PostgresDriver) DeleteBranch(ctx context.Context, branchName string) error {
	if d.db == nil {
		return fmt.Errorf("postgres connection not initialized")
	}
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
	if d.db == nil {
		return nil, fmt.Errorf("postgres connection not initialized")
	}
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
	defer func() {
		_ = rows.Close()
	}()

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
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return branches, nil
}

// Close terminates the database connection pool
func (d *PostgresDriver) Close() error {
	if d.db == nil {
		return nil
	}
	return d.db.Close()
}

func (d *PostgresDriver) formatDBName(branch string) string {
	sanitized := git.SanitizeBranchName(branch)
	if sanitized == "" || sanitized == "main" || sanitized == "master" || sanitized == "default" {
		return d.cfg.BaseDatabase
	}
	return fmt.Sprintf("%s_%s", d.cfg.BaseDatabase, sanitized)
}
