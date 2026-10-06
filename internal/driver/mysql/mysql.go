package mysql

import (
	"os"
)
import (
	"context"
	"database/sql"
	sqldriver "database/sql/driver"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"github.com/branchbase/branchbase/internal/driver"
	"github.com/branchbase/branchbase/internal/git"
)

// Config holds connection parameters for MySQL and MariaDB
type Config struct {
	Host          string
	Port          int
	User          string
	Password      string
	BaseDatabase  string
	DefaultBranch string
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
		if def, ok := params["default_branch"].(string); ok && strings.TrimSpace(def) != "" {
			cfg.DefaultBranch = def
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
		return nil, fmt.Errorf("mysql open: %w", err)
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

// CreateBranch clones sourceBranch into targetBranch by replicating tables, views, triggers, and rows.
func (d *MySQLDriver) CreateBranch(ctx context.Context, sourceBranch, targetBranch string) (retErr error) {
	if d.db == nil {
		return fmt.Errorf("mysql connection not initialized")
	}
	sourceDB := d.formatDBName(sourceBranch)
	targetDB := d.formatDBName(targetBranch)

	// Step 1: Detect Character Set & Collation from source database schema
	var charset, collation string
	schemaInfoQuery := "SELECT DEFAULT_CHARACTER_SET_NAME, DEFAULT_COLLATION_NAME FROM information_schema.SCHEMATA WHERE SCHEMA_NAME = ?"
	_ = d.db.QueryRowContext(ctx, schemaInfoQuery, sourceDB).Scan(&charset, &collation)

	createDBSQL := fmt.Sprintf("CREATE DATABASE %s;", QuoteIdentifier(targetDB))
	if charset != "" && collation != "" {
		createDBSQL = fmt.Sprintf("CREATE DATABASE %s CHARACTER SET %s COLLATE %s;", QuoteIdentifier(targetDB), QuoteIdentifier(charset), QuoteIdentifier(collation))
	}

	if _, err := d.db.ExecContext(ctx, createDBSQL); err != nil {
		// Fallback to standard CREATE DATABASE if custom charset fails
		fallbackSQL := fmt.Sprintf("CREATE DATABASE %s;", QuoteIdentifier(targetDB))
		if _, fbErr := d.db.ExecContext(ctx, fallbackSQL); fbErr != nil {
			return fmt.Errorf("failed to create database %q: %w", targetDB, err)
		}
	}
	created := true
	defer func() {
		if retErr == nil || !created {
			return
		}
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		dropSQL := fmt.Sprintf("DROP DATABASE IF EXISTS %s;", QuoteIdentifier(targetDB))
		if _, err := d.db.ExecContext(cleanupCtx, dropSQL); err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("failed to remove incomplete branch database %q: %w", targetDB, err))
		}
	}()

	// Hold one session for SET FOREIGN_KEY_CHECKS + table copies. Pool connections
	// would otherwise apply the session variable to a different connection than
	// the INSERT (Error 1452 when a child table is copied before its parent).
	conn, err := d.db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("failed to reserve mysql connection: %w", err)
	}
	defer func() {
		if err := conn.Close(); err != nil { fmt.Fprintf(os.Stderr, "failed to close conn: %v\n", err) }
	}()

	if _, err := conn.ExecContext(ctx, "SET FOREIGN_KEY_CHECKS=0"); err != nil {
		return fmt.Errorf("failed to disable foreign key checks: %w", err)
	}
	defer func() {
		resetCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if _, err := conn.ExecContext(resetCtx, "SET FOREIGN_KEY_CHECKS=1"); err != nil {
			rawErr := conn.Raw(func(any) error { return sqldriver.ErrBadConn })
			if rawErr != nil && !errors.Is(rawErr, sqldriver.ErrBadConn) {
				retErr = errors.Join(retErr, fmt.Errorf("failed to discard MySQL session after foreign key reset failure: %w", rawErr))
			}
			retErr = errors.Join(retErr, fmt.Errorf("failed to restore MySQL foreign key checks: %w", err))
		}
	}()

	// Step 2: Query tables from source database
	tableQuery := "SELECT TABLE_NAME FROM information_schema.TABLES WHERE TABLE_SCHEMA = ? AND TABLE_TYPE = 'BASE TABLE'"
	rows, err := conn.QueryContext(ctx, tableQuery, sourceDB)
	if err != nil {
		return fmt.Errorf("failed to list tables in %q: %w", sourceDB, err)
	}
	defer func() {
		if err := rows.Close(); err != nil { fmt.Fprintf(os.Stderr, "failed to close rows: %v\n", err) }
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
	if err := rows.Close(); err != nil {
		return err
	}

	// Step 3: Clone each table structure and copy data (excluding generated columns)
	for _, tbl := range tables {
		qTargetTbl := fmt.Sprintf("%s.%s", QuoteIdentifier(targetDB), QuoteIdentifier(tbl))
		qSourceTbl := fmt.Sprintf("%s.%s", QuoteIdentifier(sourceDB), QuoteIdentifier(tbl))

		createTblSQL := fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s LIKE %s;", qTargetTbl, qSourceTbl)
		if _, err := conn.ExecContext(ctx, createTblSQL); err != nil {
			return fmt.Errorf("failed to create table %q: %w", tbl, err)
		}

		// Inspect non-generated columns to prevent MySQL Error 3105 on generated columns
		colQuery := "SELECT COLUMN_NAME FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ? AND EXTRA NOT LIKE '%GENERATED%' ORDER BY ORDINAL_POSITION"
		colRows, colErr := conn.QueryContext(ctx, colQuery, sourceDB, tbl)
		if colErr != nil {
			return fmt.Errorf("failed to inspect columns for table %q: %w", tbl, colErr)
		}
		var insertCols []string
		for colRows.Next() {
			var colName string
			if err := colRows.Scan(&colName); err != nil {
				_ = colRows.Close()
				return fmt.Errorf("failed to scan column for table %q: %w", tbl, err)
			}
			insertCols = append(insertCols, QuoteIdentifier(colName))
		}
		if err := colRows.Err(); err != nil {
			_ = colRows.Close()
			return fmt.Errorf("column iteration error for table %q: %w", tbl, err)
		}
		if err := colRows.Close(); err != nil {
			return fmt.Errorf("failed to close column rows for table %q: %w", tbl, err)
		}

		var insertDataSQL string
		if len(insertCols) > 0 {
			colList := strings.Join(insertCols, ", ")
			insertDataSQL = fmt.Sprintf("INSERT INTO %s (%s) SELECT %s FROM %s;", qTargetTbl, colList, colList, qSourceTbl)
		} else {
			insertDataSQL = fmt.Sprintf("INSERT INTO %s SELECT * FROM %s;", qTargetTbl, qSourceTbl)
		}

		if _, err := conn.ExecContext(ctx, insertDataSQL); err != nil {
			return fmt.Errorf("failed to copy data for table %q: %w", tbl, err)
		}
	}

	// Step 4: Replicate Views
	viewQuery := "SELECT TABLE_NAME FROM information_schema.VIEWS WHERE TABLE_SCHEMA = ?"
	vRows, vErr := conn.QueryContext(ctx, viewQuery, sourceDB)
	if vErr != nil {
		return fmt.Errorf("failed to query views in %q: %w", sourceDB, vErr)
	}
	var views []string
	for vRows.Next() {
		var vName string
		if err := vRows.Scan(&vName); err != nil {
			_ = vRows.Close()
			return fmt.Errorf("failed to scan view name: %w", err)
		}
		views = append(views, vName)
	}
	if err := vRows.Err(); err != nil {
		_ = vRows.Close()
		return fmt.Errorf("view iteration error: %w", err)
	}
	if err := vRows.Close(); err != nil {
		return fmt.Errorf("failed to close view rows: %w", err)
	}

	for _, v := range views {
		var viewName, createViewSQL, csClient, collConn sql.NullString
		showQuery := fmt.Sprintf("SHOW CREATE VIEW %s.%s", QuoteIdentifier(sourceDB), QuoteIdentifier(v))
		if err := conn.QueryRowContext(ctx, showQuery).Scan(&viewName, &createViewSQL, &csClient, &collConn); err != nil {
			return fmt.Errorf("failed to fetch CREATE VIEW for %q: %w", v, err)
		}
		if createViewSQL.Valid {
			rewrittenSQL := strings.ReplaceAll(createViewSQL.String, QuoteIdentifier(sourceDB)+".", QuoteIdentifier(targetDB)+".")
			rewrittenSQL = strings.ReplaceAll(rewrittenSQL, sourceDB+".", targetDB+".")
			if _, err := conn.ExecContext(ctx, rewrittenSQL); err != nil {
				return fmt.Errorf("failed to create view %q in %q: %w", v, targetDB, err)
			}
		}
	}

	// Step 5: Replicate Triggers
	triggerQuery := "SELECT TRIGGER_NAME FROM information_schema.TRIGGERS WHERE TRIGGER_SCHEMA = ?"
	tRows, tErr := conn.QueryContext(ctx, triggerQuery, sourceDB)
	if tErr != nil {
		return fmt.Errorf("failed to query triggers in %q: %w", sourceDB, tErr)
	}
	var triggers []string
	for tRows.Next() {
		var trgName string
		if err := tRows.Scan(&trgName); err != nil {
			_ = tRows.Close()
			return fmt.Errorf("failed to scan trigger name: %w", err)
		}
		triggers = append(triggers, trgName)
	}
	if err := tRows.Err(); err != nil {
		_ = tRows.Close()
		return fmt.Errorf("trigger iteration error: %w", err)
	}
	if err := tRows.Close(); err != nil {
		return fmt.Errorf("failed to close trigger rows: %w", err)
	}

	if len(triggers) > 0 {
		if _, err := conn.ExecContext(ctx, fmt.Sprintf("USE %s;", QuoteIdentifier(targetDB))); err != nil {
			return fmt.Errorf("failed to select target database %q for triggers: %w", targetDB, err)
		}
		for _, trg := range triggers {
			var trgN, sqlMode, origStmt, csCl, collCo, dbColl, created sql.NullString
			showTrgQuery := fmt.Sprintf("SHOW CREATE TRIGGER %s.%s", QuoteIdentifier(sourceDB), QuoteIdentifier(trg))
			if err := conn.QueryRowContext(ctx, showTrgQuery).Scan(&trgN, &sqlMode, &origStmt, &csCl, &collCo, &dbColl, &created); err != nil {
				return fmt.Errorf("failed to fetch CREATE TRIGGER for %q: %w", trg, err)
			}
			if origStmt.Valid {
				rewrittenTrg := strings.ReplaceAll(origStmt.String, QuoteIdentifier(sourceDB)+".", QuoteIdentifier(targetDB)+".")
				rewrittenTrg = strings.ReplaceAll(rewrittenTrg, sourceDB+".", targetDB+".")
				if _, err := conn.ExecContext(ctx, rewrittenTrg); err != nil {
					return fmt.Errorf("failed to create trigger %q in %q: %w", trg, targetDB, err)
				}
			}
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
		if err := rows.Close(); err != nil { fmt.Fprintf(os.Stderr, "failed to close rows: %v\n", err) }
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
			if err := sizeRows.Close(); err != nil { fmt.Fprintf(os.Stderr, "failed to close sizeRows: %v\n", err) }
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
			branchName = d.listedDefaultBranch()
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
	if d.isBaseBranch(sanitized) {
		return d.cfg.BaseDatabase
	}
	return fmt.Sprintf("%s_%s", d.cfg.BaseDatabase, sanitized)
}

func (d *MySQLDriver) isBaseBranch(sanitized string) bool {
	def := strings.TrimSpace(d.cfg.DefaultBranch)
	if def == "" {
		return sanitized == "main" || sanitized == "master" || sanitized == "default"
	}
	return sanitized == git.SanitizeBranchName(def)
}

func (d *MySQLDriver) listedDefaultBranch() string {
	def := strings.TrimSpace(d.cfg.DefaultBranch)
	if def == "" {
		return "main"
	}
	return git.SanitizeBranchName(def)
}
