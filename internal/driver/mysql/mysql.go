package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/branchbase/branchbase/internal/driver"
	"github.com/branchbase/branchbase/internal/git"
)

// Config holds connection parameters for MySQL and MariaDB
type Config struct {
	Host         string
	Port         int
	User         string
	Password     string
	BaseDatabase string
}

// PoolConfig controls the database connection pool used by a MySQLDriver.
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

// QuoteIdentifier safely wraps a MySQL identifier in backticks with escaping.
func QuoteIdentifier(name string) string {
	escaped := strings.ReplaceAll(name, "`", "``")
	return "`" + escaped + "`"
}

// DSN returns the MySQL Data Source Name connection string
func (c Config) DSN() string {
	host := c.Host
	if host == "" {
		host = "127.0.0.1"
	}
	port := c.Port
	if port <= 0 {
		port = 3306
	}

	auth := c.User
	if auth == "" {
		auth = "root"
	}
	if c.Password != "" {
		auth = fmt.Sprintf("%s:%s", auth, c.Password)
	}

	dbName := c.BaseDatabase
	if dbName == "" {
		dbName = "mysql"
	}

	return fmt.Sprintf("%s@tcp(%s:%d)/%s?parseTime=true&multiStatements=true", auth, host, port, dbName)
}

// MySQLDriver manages database branching for MySQL and MariaDB engines
type MySQLDriver struct {
	cfg Config
	db  *sql.DB
}

func init() {
	factory := func(params map[string]interface{}) (driver.Driver, error) {
		cfg := Config{
			Host:         "127.0.0.1",
			Port:         3306,
			User:         "root",
			BaseDatabase: "myapp_dev",
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

		return NewWithPoolConfig(cfg, poolConfigFromParams(params))
	}

	driver.Register("mysql", factory)
	driver.Register("mariadb", factory)
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

// New creates a new MySQLDriver instance and initializes the connection pool
func New(cfg Config) (*MySQLDriver, error) {
	return NewWithPoolConfig(cfg, defaultPoolConfig)
}

// NewWithPoolConfig creates a MySQLDriver with supplied pool settings
func NewWithPoolConfig(cfg Config, poolConfig PoolConfig) (*MySQLDriver, error) {
	db, err := sql.Open("mysql", cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("failed to open mysql connection: %w", err)
	}

	db.SetMaxOpenConns(poolConfig.MaxOpenConns)
	db.SetMaxIdleConns(poolConfig.MaxIdleConns)
	db.SetConnMaxLifetime(poolConfig.ConnMaxLifetime)

	return &MySQLDriver{
		cfg: cfg,
		db:  db,
	}, nil
}

// NewWithDB creates a MySQLDriver with an existing database handle (useful for unit tests and sqlmock)
func NewWithDB(cfg Config, db *sql.DB) *MySQLDriver {
	return &MySQLDriver{
		cfg: cfg,
		db:  db,
	}
}

// DB returns the underlying *sql.DB connection pool
func (d *MySQLDriver) DB() *sql.DB {
	return d.db
}

func (d *MySQLDriver) Name() string {
	return "mysql"
}

func (d *MySQLDriver) Ping(ctx context.Context) error {
	if d.db == nil {
		return fmt.Errorf("mysql connection not initialized")
	}
	return d.db.PingContext(ctx)
}

func (d *MySQLDriver) BranchExists(ctx context.Context, branchName string) (bool, error) {
	if d.db == nil {
		return false, fmt.Errorf("mysql connection not initialized")
	}
	dbName := d.formatDBName(branchName)
	query := "SELECT 1 FROM information_schema.SCHEMATA WHERE SCHEMA_NAME = ?"

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

// CreateBranch clones sourceBranch into targetBranch by replicating tables and rows.
func (d *MySQLDriver) CreateBranch(ctx context.Context, sourceBranch, targetBranch string) error {
	if d.db == nil {
		return fmt.Errorf("mysql connection not initialized")
	}
	sourceDB := d.formatDBName(sourceBranch)
	targetDB := d.formatDBName(targetBranch)

	// Step 1: Create target database schema
	createDBSQL := fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s;", QuoteIdentifier(targetDB))
	if _, err := d.db.ExecContext(ctx, createDBSQL); err != nil {
		return fmt.Errorf("failed to create database %q: %w", targetDB, err)
	}

	// Step 2: Query tables from source database
	tableQuery := "SELECT TABLE_NAME FROM information_schema.TABLES WHERE TABLE_SCHEMA = ? AND TABLE_TYPE = 'BASE TABLE'"
	rows, err := d.db.QueryContext(ctx, tableQuery, sourceDB)
	if err != nil {
		return fmt.Errorf("failed to list tables in %q: %w", sourceDB, err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var tables []string
	for rows.Next() {
		var tbl string
		if err := rows.Scan(&tbl); err != nil {
			return err
		}
		tables = append(tables, tbl)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	// Step 3: Clone each table structure and copy data
	for _, tbl := range tables {
		qTargetTbl := fmt.Sprintf("%s.%s", QuoteIdentifier(targetDB), QuoteIdentifier(tbl))
		qSourceTbl := fmt.Sprintf("%s.%s", QuoteIdentifier(sourceDB), QuoteIdentifier(tbl))

		createTblSQL := fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s LIKE %s;", qTargetTbl, qSourceTbl)
		if _, err := d.db.ExecContext(ctx, createTblSQL); err != nil {
			return fmt.Errorf("failed to create table %q: %w", tbl, err)
		}

		insertDataSQL := fmt.Sprintf("INSERT INTO %s SELECT * FROM %s;", qTargetTbl, qSourceTbl)
		if _, err := d.db.ExecContext(ctx, insertDataSQL); err != nil {
			return fmt.Errorf("failed to copy data for table %q: %w", tbl, err)
		}
	}

	return nil
}

// DeleteBranch drops the specified branch database
func (d *MySQLDriver) DeleteBranch(ctx context.Context, branchName string) error {
	if d.db == nil {
		return fmt.Errorf("mysql connection not initialized")
	}
	dbName := d.formatDBName(branchName)

	if dbName == d.cfg.BaseDatabase {
		return fmt.Errorf("cannot delete protected base database %q", dbName)
	}

	dropQuery := fmt.Sprintf("DROP DATABASE IF EXISTS %s;", QuoteIdentifier(dbName))
	_, err := d.db.ExecContext(ctx, dropQuery)
	return err
}

// escapeLikeWildcards escapes '\', '%', and '_' characters for MySQL LIKE queries
func escapeLikeWildcards(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

// ListBranches returns all databases managed by BranchBase for this MySQL instance
func (d *MySQLDriver) ListBranches(ctx context.Context) ([]driver.BranchInfo, error) {
	if d.db == nil {
		return nil, fmt.Errorf("mysql connection not initialized")
	}

	exactBase := d.cfg.BaseDatabase
	branchPattern := escapeLikeWildcards(d.cfg.BaseDatabase) + `\_%`

	// Query Schemas
	schemaQuery := `
		SELECT SCHEMA_NAME
		FROM information_schema.SCHEMATA
		WHERE SCHEMA_NAME = ? OR SCHEMA_NAME LIKE ?
		ORDER BY SCHEMA_NAME ASC;
	`
	rows, err := d.db.QueryContext(ctx, schemaQuery, exactBase, branchPattern)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	var schemas []string
	for rows.Next() {
		var schema string
		if err := rows.Scan(&schema); err != nil {
			return nil, err
		}
		schemas = append(schemas, schema)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Query sizes for each schema
	sizeQuery := `
		SELECT table_schema, COALESCE(SUM(data_length + index_length), 0)
		FROM information_schema.TABLES
		WHERE table_schema = ? OR table_schema LIKE ?
		GROUP BY table_schema;
	`
	sizeRows, err := d.db.QueryContext(ctx, sizeQuery, exactBase, branchPattern)
	sizes := make(map[string]int64)
	if err == nil {
		defer func() {
			_ = sizeRows.Close()
		}()
		for sizeRows.Next() {
			var sch string
			var sz int64
			if err := sizeRows.Scan(&sch, &sz); err == nil {
				sizes[sch] = sz
			}
		}
	}

	var branches []driver.BranchInfo
	for _, datname := range schemas {
		branchName := strings.TrimPrefix(datname, d.cfg.BaseDatabase+"_")
		if datname == d.cfg.BaseDatabase {
			branchName = "main"
		}

		branches = append(branches, driver.BranchInfo{
			Name:        branchName,
			Database:    datname,
			CreatedAt:   time.Now(),
			SizeBytes:   sizes[datname],
			IsProtected: datname == d.cfg.BaseDatabase,
		})
	}

	return branches, nil
}

// Close closes the underlying MySQL connection pool
func (d *MySQLDriver) Close() error {
	if d.db == nil {
		return nil
	}
	return d.db.Close()
}

func (d *MySQLDriver) formatDBName(branch string) string {
	sanitized := git.SanitizeBranchName(branch)
	if sanitized == "" || sanitized == "main" || sanitized == "master" || sanitized == "default" {
		return d.cfg.BaseDatabase
	}
	return fmt.Sprintf("%s_%s", d.cfg.BaseDatabase, sanitized)
}
