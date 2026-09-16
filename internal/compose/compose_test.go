package compose

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/branchbase/branchbase/internal/config"
)

func TestDetectComposePostgres(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	content := `
version: '3.8'
services:
  db:
    image: postgres:15-alpine
    ports:
      - "5433:5432"
    environment:
      POSTGRES_USER: devuser
      POSTGRES_PASSWORD: secretpassword
      POSTGRES_DB: production_dev
`
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(content), 0644); err != nil {
		t.Fatalf("failed to write compose file: %v", err)
	}

	dbSvc, filename, err := DetectCompose(dir)
	if err != nil {
		t.Fatalf("DetectCompose failed: %v", err)
	}
	if dbSvc == nil {
		t.Fatal("expected dbSvc not to be nil")
	}
	if filename != "docker-compose.yml" {
		t.Errorf("filename = %q, want docker-compose.yml", filename)
	}
	if dbSvc.Driver != "postgres" {
		t.Errorf("driver = %q, want postgres", dbSvc.Driver)
	}
	if dbSvc.Port != 5433 {
		t.Errorf("port = %d, want 5433", dbSvc.Port)
	}
	if dbSvc.User != "devuser" {
		t.Errorf("user = %q, want devuser", dbSvc.User)
	}
	if dbSvc.Password != "secretpassword" {
		t.Errorf("password = %q, want secretpassword", dbSvc.Password)
	}
	if dbSvc.Database != "production_dev" {
		t.Errorf("database = %q, want production_dev", dbSvc.Database)
	}

	cfg := config.DefaultConfig()
	ApplyToConfig(&cfg, dbSvc)
	if cfg.Driver != "postgres" || cfg.Connection.Port != 5433 || cfg.Connection.BaseDatabase != "production_dev" {
		t.Errorf("ApplyToConfig did not populate expected fields: %+v", cfg)
	}
}

func TestDetectComposeMySQL(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	content := `
services:
  mysql_db:
    image: mysql:8.0
    ports:
      - "127.0.0.1:3307:3306"
    environment:
      - MYSQL_USER=appuser
      - MYSQL_PASSWORD=appsecret
      - MYSQL_DATABASE=shop_dev
`
	if err := os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte(content), 0644); err != nil {
		t.Fatalf("failed to write compose file: %v", err)
	}

	dbSvc, filename, err := DetectCompose(dir)
	if err != nil {
		t.Fatalf("DetectCompose failed: %v", err)
	}
	if dbSvc == nil {
		t.Fatal("expected dbSvc not to be nil")
	}
	if filename != "compose.yaml" {
		t.Errorf("filename = %q, want compose.yaml", filename)
	}
	if dbSvc.Driver != "mysql" {
		t.Errorf("driver = %q, want mysql", dbSvc.Driver)
	}
	if dbSvc.Port != 3307 {
		t.Errorf("port = %d, want 3307", dbSvc.Port)
	}
	if dbSvc.User != "appuser" {
		t.Errorf("user = %q, want appuser", dbSvc.User)
	}
	if dbSvc.Database != "shop_dev" {
		t.Errorf("database = %q, want shop_dev", dbSvc.Database)
	}
}

func TestDetectComposeNone(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	dbSvc, filename, err := DetectCompose(dir)
	if err != nil {
		t.Fatalf("DetectCompose failed: %v", err)
	}
	if dbSvc != nil || filename != "" {
		t.Fatalf("expected nil for empty dir, got %+v %q", dbSvc, filename)
	}
}
