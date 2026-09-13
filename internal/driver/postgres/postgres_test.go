package postgres

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/branchbase/branchbase/internal/driver"
)

func TestFormatDBName(t *testing.T) {
	t.Parallel()
	d, err := New(Config{
		BaseDatabase: "myapp_dev",
	})
	if err != nil {
		t.Fatalf("unexpected error creating driver: %v", err)
	}
	defer func() { _ = d.Close() }()

	tests := []struct {
		branch   string
		expected string
	}{
		{"main", "myapp_dev"},
		{"master", "myapp_dev"},
		{"", "myapp_dev"},
		{"   ", "myapp_dev"},
		{"feature_billing", "myapp_dev_feature_billing"},
		{"hotfix_auth", "myapp_dev_hotfix_auth"},
	}

	for _, tt := range tests {
		t.Run(tt.branch, func(t *testing.T) {
			t.Parallel()
			got := d.formatDBName(tt.branch)
			if got != tt.expected {
				t.Errorf("formatDBName(%q) = %q; want %q", tt.branch, got, tt.expected)
			}
		})
	}
}

func TestDriverName(t *testing.T) {
	t.Parallel()
	d, err := New(Config{BaseDatabase: "myapp_dev"})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer func() { _ = d.Close() }()

	if d.Name() != "postgres" {
		t.Errorf("expected driver name 'postgres', got %q", d.Name())
	}
}

func TestDeleteBranchProtectedBaseDatabase(t *testing.T) {
	t.Parallel()
	d, err := New(Config{BaseDatabase: "myapp_dev"})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer func() { _ = d.Close() }()
	ctx := context.Background()

	protectedBranches := []string{"main", "master", "", "   "}
	for _, branch := range protectedBranches {
		err := d.DeleteBranch(ctx, branch)
		if err == nil {
			t.Errorf("expected error deleting protected branch %q, got nil", branch)
		}
		if !strings.Contains(err.Error(), "cannot delete protected base database") {
			t.Errorf("expected protected database error for branch %q, got: %v", branch, err)
		}
	}
}

func TestNewInitializesDB(t *testing.T) {
	t.Parallel()
	d, err := New(Config{
		Host:         "127.0.0.1",
		Port:         5432,
		User:         "postgres",
		BaseDatabase: "myapp_dev",
		SSLMode:      "disable",
	})
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}
	defer func() { _ = d.Close() }()

	if d.DB() == nil {
		t.Fatal("expected non-nil *sql.DB from New(), got nil")
	}
}

func TestNilDBGuards(t *testing.T) {
	t.Parallel()
	d := NewWithDB(Config{BaseDatabase: "myapp_dev"}, nil)
	ctx := context.Background()

	if err := d.Ping(ctx); err == nil || !strings.Contains(err.Error(), "not initialized") {
		t.Errorf("Ping on nil db: expected not initialized error, got: %v", err)
	}

	if _, err := d.BranchExists(ctx, "feature"); err == nil || !strings.Contains(err.Error(), "not initialized") {
		t.Errorf("BranchExists on nil db: expected not initialized error, got: %v", err)
	}

	if err := d.CreateBranch(ctx, "main", "feature"); err == nil || !strings.Contains(err.Error(), "not initialized") {
		t.Errorf("CreateBranch on nil db: expected not initialized error, got: %v", err)
	}

	if err := d.DeleteBranch(ctx, "feature"); err == nil || !strings.Contains(err.Error(), "not initialized") {
		t.Errorf("DeleteBranch on nil db: expected not initialized error, got: %v", err)
	}

	if _, err := d.ListBranches(ctx); err == nil || !strings.Contains(err.Error(), "not initialized") {
		t.Errorf("ListBranches on nil db: expected not initialized error, got: %v", err)
	}

	if err := d.Close(); err != nil {
		t.Errorf("Close on nil db: expected nil, got: %v", err)
	}
}

func TestPingWithMock(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer func() { _ = db.Close() }()

	d := NewWithDB(Config{BaseDatabase: "myapp_dev"}, db)
	ctx := context.Background()

	// Happy path
	mock.ExpectPing()
	if err := d.Ping(ctx); err != nil {
		t.Errorf("Ping() unexpected error: %v", err)
	}

	// Error path
	mock.ExpectPing().WillReturnError(errors.New("db unreachable"))
	if err := d.Ping(ctx); err == nil {
		t.Error("Ping() expected error, got nil")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled mock expectations: %v", err)
	}
}

func TestBranchExistsWithMock(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer func() { _ = db.Close() }()

	d := NewWithDB(Config{BaseDatabase: "myapp_dev"}, db)
	ctx := context.Background()

	// Case 1: Branch exists
	mock.ExpectQuery(`SELECT 1 FROM pg_database WHERE datname = \$1`).
		WithArgs("myapp_dev_feature_billing").
		WillReturnRows(sqlmock.NewRows([]string{"?column?"}).AddRow(1))

	exists, err := d.BranchExists(ctx, "feature_billing")
	if err != nil {
		t.Fatalf("BranchExists failed: %v", err)
	}
	if !exists {
		t.Errorf("expected branch to exist, got false")
	}

	// Case 2: Branch does not exist (sql.ErrNoRows)
	mock.ExpectQuery(`SELECT 1 FROM pg_database WHERE datname = \$1`).
		WithArgs("myapp_dev_nonexistent").
		WillReturnError(sql.ErrNoRows)

	exists, err = d.BranchExists(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("BranchExists failed: %v", err)
	}
	if exists {
		t.Errorf("expected branch to not exist, got true")
	}

	// Case 3: DB query error
	mock.ExpectQuery(`SELECT 1 FROM pg_database WHERE datname = \$1`).
		WithArgs("myapp_dev_error").
		WillReturnError(errors.New("connection broken"))

	_, err = d.BranchExists(ctx, "error")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled mock expectations: %v", err)
	}
}

func TestCreateBranchWithMock(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer func() { _ = db.Close() }()

	d := NewWithDB(Config{BaseDatabase: "myapp_dev"}, db)
	ctx := context.Background()

	// Case 1: Success
	mock.ExpectExec(`SELECT pg_terminate_backend\(pid\)`).
		WithArgs("myapp_dev").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`CREATE DATABASE "myapp_dev_feature_auth" TEMPLATE "myapp_dev";`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	if err := d.CreateBranch(ctx, "main", "feature_auth"); err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}

	// Case 2: Create query fails
	mock.ExpectExec(`SELECT pg_terminate_backend\(pid\)`).
		WithArgs("myapp_dev").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`CREATE DATABASE "myapp_dev_feature_auth" TEMPLATE "myapp_dev";`).
		WillReturnError(errors.New("source database is being accessed by other users"))

	if err := d.CreateBranch(ctx, "main", "feature_auth"); err == nil {
		t.Fatal("expected error on CreateBranch, got nil")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled mock expectations: %v", err)
	}
}

func TestDeleteBranchWithMock(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer func() { _ = db.Close() }()

	d := NewWithDB(Config{BaseDatabase: "myapp_dev"}, db)
	ctx := context.Background()

	// Case 1: Success
	mock.ExpectExec(`SELECT pg_terminate_backend\(pid\)`).
		WithArgs("myapp_dev_feature_auth").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DROP DATABASE IF EXISTS "myapp_dev_feature_auth";`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	if err := d.DeleteBranch(ctx, "feature_auth"); err != nil {
		t.Fatalf("DeleteBranch failed: %v", err)
	}

	// Case 2: Drop failure
	mock.ExpectExec(`SELECT pg_terminate_backend\(pid\)`).
		WithArgs("myapp_dev_feature_auth").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DROP DATABASE IF EXISTS "myapp_dev_feature_auth";`).
		WillReturnError(errors.New("database locked"))

	if err := d.DeleteBranch(ctx, "feature_auth"); err == nil {
		t.Fatal("expected error on DeleteBranch, got nil")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled mock expectations: %v", err)
	}
}

func TestListBranchesWithMock(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer func() { _ = db.Close() }()

	d := NewWithDB(Config{BaseDatabase: "myapp_dev"}, db)
	ctx := context.Background()

	// Case 1: Success
	mock.ExpectQuery(`SELECT datname, pg_database_size\(datname\) FROM pg_database WHERE datname LIKE \$1 ORDER BY datname ASC;`).
		WithArgs("myapp_dev%").
		WillReturnRows(sqlmock.NewRows([]string{"datname", "pg_database_size"}).
			AddRow("myapp_dev", int64(10485760)).
			AddRow("myapp_dev_feature_auth", int64(20971520)))

	branches, err := d.ListBranches(ctx)
	if err != nil {
		t.Fatalf("ListBranches failed: %v", err)
	}
	if len(branches) != 2 {
		t.Fatalf("expected 2 branches, got %d", len(branches))
	}

	if branches[0].Name != "main" || !branches[0].IsProtected || branches[0].SizeBytes != 10485760 {
		t.Errorf("unexpected branch[0]: %+v", branches[0])
	}
	if branches[1].Name != "feature_auth" || branches[1].IsProtected || branches[1].SizeBytes != 20971520 {
		t.Errorf("unexpected branch[1]: %+v", branches[1])
	}

	// Case 2: Query failure
	mock.ExpectQuery(`SELECT datname, pg_database_size\(datname\) FROM pg_database WHERE datname LIKE \$1 ORDER BY datname ASC;`).
		WithArgs("myapp_dev%").
		WillReturnError(errors.New("permission denied"))

	if _, err := d.ListBranches(ctx); err == nil {
		t.Fatal("expected error on ListBranches, got nil")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled mock expectations: %v", err)
	}
}

func TestCloseWithMock(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}

	d := NewWithDB(Config{BaseDatabase: "myapp_dev"}, db)
	mock.ExpectClose()

	if err := d.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled mock expectations: %v", err)
	}
}

func TestDSN(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		cfg      Config
		expected string
	}{
		{
			name: "full config with password",
			cfg: Config{
				Host:         "db.internal",
				Port:         5433,
				User:         "admin",
				Password:     "secret123",
				BaseDatabase: "prod_db",
				SSLMode:      "require",
			},
			expected: "postgres://admin:secret123@db.internal:5433/prod_db?sslmode=require",
		},
		{
			name: "defaults applied",
			cfg: Config{
				BaseDatabase: "myapp_dev",
			},
			expected: "postgres://127.0.0.1:5432/myapp_dev?sslmode=disable",
		},
		{
			name: "user without password",
			cfg: Config{
				Host:         "localhost",
				Port:         5432,
				User:         "postgres",
				BaseDatabase: "dev_db",
				SSLMode:      "disable",
			},
			expected: "postgres://postgres@localhost:5432/dev_db?sslmode=disable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tt.cfg.DSN()
			if got != tt.expected {
				t.Errorf("DSN() = %q; want %q", got, tt.expected)
			}
		})
	}
}

func TestNewDefaultSSLMode(t *testing.T) {
	t.Parallel()
	d, err := New(Config{BaseDatabase: "test_db", SSLMode: ""})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer func() { _ = d.Close() }()

	if d.cfg.SSLMode != "disable" {
		t.Errorf("expected default SSLMode 'disable', got %q", d.cfg.SSLMode)
	}
}

func TestPostgresDriverRegistered(t *testing.T) {
	t.Parallel()
	drv, err := driver.GetDriver("postgres", map[string]interface{}{
		"base_database": "custom_db",
		"port":          5432,
	})
	if err != nil {
		t.Fatalf("GetDriver failed: %v", err)
	}
	defer func() { _ = drv.Close() }()

	if drv.Name() != "postgres" {
		t.Errorf("expected 'postgres', got %q", drv.Name())
	}
}
