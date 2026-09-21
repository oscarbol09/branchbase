package mysql

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/branchbase/branchbase/internal/driver"
)

func TestMySQLQuoteIdentifier(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   string
		want string
	}{
		{"users", "`users`"},
		{"app`db", "`app``db`"},
		{"myapp_dev_feature_x", "`myapp_dev_feature_x`"},
	}
	for _, tt := range tests {
		if got := QuoteIdentifier(tt.in); got != tt.want {
			t.Errorf("QuoteIdentifier(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestMySQLDSN(t *testing.T) {
	t.Parallel()
	cfg := Config{
		Host:         "127.0.0.1",
		Port:         3306,
		User:         "admin",
		Password:     "secret",
		BaseDatabase: "myapp_dev",
	}
	want := "admin:secret@tcp(127.0.0.1:3306)/myapp_dev?parseTime=true&multiStatements=true"
	if got := cfg.DSN(); got != want {
		t.Errorf("DSN() = %q, want %q", got, want)
	}
}

func TestMySQLBranchExists(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	defer func() {
		_ = db.Close()
	}()

	drv := NewWithDB(Config{BaseDatabase: "myapp_dev"}, db)
	ctx := context.Background()

	// 1. Exists case
	mock.ExpectQuery(regexp.QuoteMeta("SELECT 1 FROM information_schema.SCHEMATA WHERE SCHEMA_NAME = ?")).
		WithArgs("myapp_dev_feature_a").
		WillReturnRows(sqlmock.NewRows([]string{"1"}).AddRow(1))

	exists, err := drv.BranchExists(ctx, "feature-a")
	if err != nil || !exists {
		t.Fatalf("expected exists=true, got exists=%v err=%v", exists, err)
	}

	// 2. Not exists case
	mock.ExpectQuery(regexp.QuoteMeta("SELECT 1 FROM information_schema.SCHEMATA WHERE SCHEMA_NAME = ?")).
		WithArgs("myapp_dev_nonexistent").
		WillReturnRows(sqlmock.NewRows([]string{"1"}))

	exists, err = drv.BranchExists(ctx, "nonexistent")
	if err != nil || exists {
		t.Fatalf("expected exists=false, got exists=%v err=%v", exists, err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func TestMySQLCreateBranch(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	defer func() {
		_ = db.Close()
	}()

	drv := NewWithDB(Config{BaseDatabase: "myapp_dev"}, db)
	ctx := context.Background()

	// Expect create database
	mock.ExpectExec(regexp.QuoteMeta("CREATE DATABASE IF NOT EXISTS `myapp_dev_feature_b`;")).
		WillReturnResult(sqlmock.NewResult(1, 1))

	mock.ExpectExec(regexp.QuoteMeta("SET FOREIGN_KEY_CHECKS=0")).
		WillReturnResult(sqlmock.NewResult(0, 0))

	// Expect list tables in source database
	mock.ExpectQuery(regexp.QuoteMeta("SELECT TABLE_NAME FROM information_schema.TABLES WHERE TABLE_SCHEMA = ? AND TABLE_TYPE = 'BASE TABLE'")).
		WithArgs("myapp_dev").
		WillReturnRows(sqlmock.NewRows([]string{"TABLE_NAME"}).AddRow("orders").AddRow("users"))

	// Child table first: copy must still succeed with FOREIGN_KEY_CHECKS off.
	mock.ExpectExec(regexp.QuoteMeta("CREATE TABLE IF NOT EXISTS `myapp_dev_feature_b`.`orders` LIKE `myapp_dev`.`orders`;")).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `myapp_dev_feature_b`.`orders` SELECT * FROM `myapp_dev`.`orders`;")).
		WillReturnResult(sqlmock.NewResult(1, 1))

	mock.ExpectExec(regexp.QuoteMeta("CREATE TABLE IF NOT EXISTS `myapp_dev_feature_b`.`users` LIKE `myapp_dev`.`users`;")).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `myapp_dev_feature_b`.`users` SELECT * FROM `myapp_dev`.`users`;")).
		WillReturnResult(sqlmock.NewResult(1, 1))

	mock.ExpectExec(regexp.QuoteMeta("SET FOREIGN_KEY_CHECKS=1")).
		WillReturnResult(sqlmock.NewResult(0, 0))

	if err := drv.CreateBranch(ctx, "main", "feature/b"); err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func TestMySQLCreateBranchRestoresForeignKeyChecksOnInsertError(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	defer func() {
		_ = db.Close()
	}()

	drv := NewWithDB(Config{BaseDatabase: "myapp_dev"}, db)
	ctx := context.Background()
	insertErr := errors.New("Error 1452: Cannot add or update a child row: a foreign key constraint fails")

	mock.ExpectExec(regexp.QuoteMeta("CREATE DATABASE IF NOT EXISTS `myapp_dev_feature_b`;")).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(regexp.QuoteMeta("SET FOREIGN_KEY_CHECKS=0")).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT TABLE_NAME FROM information_schema.TABLES WHERE TABLE_SCHEMA = ? AND TABLE_TYPE = 'BASE TABLE'")).
		WithArgs("myapp_dev").
		WillReturnRows(sqlmock.NewRows([]string{"TABLE_NAME"}).AddRow("orders"))
	mock.ExpectExec(regexp.QuoteMeta("CREATE TABLE IF NOT EXISTS `myapp_dev_feature_b`.`orders` LIKE `myapp_dev`.`orders`;")).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `myapp_dev_feature_b`.`orders` SELECT * FROM `myapp_dev`.`orders`;")).
		WillReturnError(insertErr)
	mock.ExpectExec(regexp.QuoteMeta("SET FOREIGN_KEY_CHECKS=1")).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err = drv.CreateBranch(ctx, "main", "feature/b")
	if err == nil {
		t.Fatal("expected CreateBranch error, got nil")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func TestMySQLDeleteBranchProtectedGuard(t *testing.T) {
	t.Parallel()
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	defer func() {
		_ = db.Close()
	}()

	drv := NewWithDB(Config{BaseDatabase: "myapp_dev"}, db)
	ctx := context.Background()

	// Deleting protected main base database must return error
	err = drv.DeleteBranch(ctx, "main")
	if err == nil {
		t.Fatal("expected error when deleting protected base database, got nil")
	}
}

func TestMySQLListBranches(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	defer func() {
		_ = db.Close()
	}()

	drv := NewWithDB(Config{BaseDatabase: "myapp_dev"}, db)
	ctx := context.Background()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT SCHEMA_NAME FROM information_schema.SCHEMATA WHERE SCHEMA_NAME = ? OR SCHEMA_NAME LIKE ? ORDER BY SCHEMA_NAME ASC;")).
		WithArgs("myapp_dev", `myapp\_dev\_%`).
		WillReturnRows(sqlmock.NewRows([]string{"SCHEMA_NAME"}).
			AddRow("myapp_dev").
			AddRow("myapp_dev_feature_a"))

	mock.ExpectQuery(regexp.QuoteMeta("SELECT table_schema, COALESCE(SUM(data_length + index_length), 0) FROM information_schema.TABLES WHERE table_schema = ? OR table_schema LIKE ? GROUP BY table_schema;")).
		WithArgs("myapp_dev", `myapp\_dev\_%`).
		WillReturnRows(sqlmock.NewRows([]string{"table_schema", "size"}).
			AddRow("myapp_dev", 1048576).
			AddRow("myapp_dev_feature_a", 2097152))

	branches, err := drv.ListBranches(ctx)
	if err != nil {
		t.Fatalf("ListBranches failed: %v", err)
	}

	if len(branches) != 2 {
		t.Fatalf("expected 2 branches, got %d", len(branches))
	}

	if branches[0].Name != "main" || !branches[0].IsProtected || branches[0].SizeBytes != 1048576 {
		t.Errorf("unexpected branch[0]: %+v", branches[0])
	}
	if branches[1].Name != "feature_a" || branches[1].IsProtected || branches[1].SizeBytes != 2097152 {
		t.Errorf("unexpected branch[1]: %+v", branches[1])
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func TestMySQLDriverRegistration(t *testing.T) {
	t.Parallel()
	drv, err := driver.GetDriver("mysql", map[string]interface{}{
		"host": "127.0.0.1",
		"port": 3306,
	})
	if err != nil {
		t.Fatalf("GetDriver('mysql') failed: %v", err)
	}
	if drv.Name() != "mysql" {
		t.Errorf("drv.Name() = %q, want mysql", drv.Name())
	}
	_ = drv.Close()

	mariaDrv, err := driver.GetDriver("mariadb", map[string]interface{}{
		"host": "127.0.0.1",
		"port": 3306,
	})
	if err != nil {
		t.Fatalf("GetDriver('mariadb') failed: %v", err)
	}
	_ = mariaDrv.Close()
}

func TestMySQLDatabaseSQLDriverRegistered(t *testing.T) {
	t.Parallel()
	for _, name := range sql.Drivers() {
		if name == "mysql" {
			return
		}
	}
	t.Fatal(`database/sql driver "mysql" is not registered`)
}

func TestNewWithPoolConfigInitializesPool(t *testing.T) {
	t.Parallel()
	drv, err := NewWithPoolConfig(Config{
		Host:         "127.0.0.1",
		Port:         3306,
		User:         "root",
		BaseDatabase: "myapp_dev",
	}, defaultPoolConfig)
	if err != nil {
		t.Fatalf("NewWithPoolConfig: %v", err)
	}
	t.Cleanup(func() { _ = drv.Close() })
	if drv.DB() == nil {
		t.Fatal("expected initialized *sql.DB, got nil")
	}
}
