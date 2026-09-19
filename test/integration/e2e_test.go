//go:build integration

package integration

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/branchbase/branchbase/internal/config"
	"github.com/branchbase/branchbase/internal/driver/postgres"
	"github.com/branchbase/branchbase/internal/proxy"
	_ "github.com/lib/pq"
)

func TestPostgreSQLE2EIntegration(t *testing.T) {
	pgHost := os.Getenv("PGHOST")
	if pgHost == "" {
		pgHost = "127.0.0.1"
	}
	pgPort := 5433
	if p := os.Getenv("PGPORT"); p != "" {
		_, _ = fmt.Sscanf(p, "%d", &pgPort)
	}

	// 1. Verify connection to live PostgreSQL container
	baseDSN := fmt.Sprintf("postgres://postgres:postgres@%s:%d/postgres?sslmode=disable", pgHost, pgPort)
	db, err := sql.Open("postgres", baseDSN)
	if err != nil {
		t.Fatalf("failed to connect to postgres: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		t.Fatalf("postgres ping failed: %v", err)
	}

	// Prepare base database
	_, _ = db.Exec("DROP DATABASE IF EXISTS myapp_dev;")
	_, _ = db.Exec("DROP DATABASE IF EXISTS myapp_dev_feature_payments;")
	_, err = db.Exec("CREATE DATABASE myapp_dev;")
	if err != nil {
		t.Fatalf("failed to create base database: %v", err)
	}

	// Seed table in base database
	appDSN := fmt.Sprintf("postgres://postgres:postgres@%s:%d/myapp_dev?sslmode=disable", pgHost, pgPort)
	appDB, err := sql.Open("postgres", appDSN)
	if err != nil {
		t.Fatalf("failed to connect to appDB: %v", err)
	}
	defer appDB.Close()

	_, err = appDB.Exec("CREATE TABLE users (id SERIAL PRIMARY KEY, name TEXT); INSERT INTO users (name) VALUES ('Alice');")
	if err != nil {
		t.Fatalf("failed to seed users table: %v", err)
	}

	// 2. Start BranchBase Proxy
	tempDir := t.TempDir()
	gitDir := filepath.Join(tempDir, ".git")
	_ = os.MkdirAll(gitDir, 0755)
	_ = os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/main\n"), 0644)

	cfg := config.DefaultConfig()
	cfg.Connection.Host = pgHost
	cfg.Connection.Port = pgPort
	cfg.Connection.BaseDatabase = "myapp_dev"
	cfg.Proxy.ListenPort = 0 // Ephemeral proxy port

	pgDrv, err := postgres.New(postgres.Config{
		Host:         pgHost,
		Port:         pgPort,
		User:         "postgres",
		Password:     "postgres",
		BaseDatabase: "myapp_dev",
		SSLMode:      "disable",
	})
	if err != nil {
		t.Fatalf("postgres driver init failed: %v", err)
	}
	defer pgDrv.Close()

	srv := proxy.NewServer(&cfg, tempDir, pgDrv)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := srv.Start(ctx); err != nil {
		t.Fatalf("proxy.Start failed: %v", err)
	}
	defer func() { _ = srv.Stop() }()

	// 3. Switch branch to feature/payments
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/feature/payments\n"), 0644); err != nil {
		t.Fatalf("failed to update git HEAD: %v", err)
	}

	// JIT create branch database
	if err := pgDrv.CreateBranch(ctx, "main", "feature/payments"); err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}

	// Verify database was cloned with data
	featDSN := fmt.Sprintf("postgres://postgres:postgres@%s:%d/myapp_dev_feature_payments?sslmode=disable", pgHost, pgPort)
	featDB, err := sql.Open("postgres", featDSN)
	if err != nil {
		t.Fatalf("failed to connect to featDB: %v", err)
	}
	defer featDB.Close()

	var count int
	err = featDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&count)
	if err != nil || count != 1 {
		t.Fatalf("expected 1 cloned user in feature branch database, got count=%d, err=%v", count, err)
	}
}
