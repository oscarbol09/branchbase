package compose

import (
	"os"
	"path/filepath"
	"strings"
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

func TestDetectComposeLongSyntaxPorts(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	content := `
services:
  db:
    image: postgres:16
    ports:
      - target: 5432
        published: 5433
        protocol: tcp
    environment:
      POSTGRES_DB: long_dev
`
	if err := os.WriteFile(filepath.Join(dir, "compose.yml"), []byte(content), 0644); err != nil {
		t.Fatalf("failed to write compose file: %v", err)
	}

	dbSvc, filename, err := DetectCompose(dir)
	if err != nil {
		t.Fatalf("DetectCompose failed: %v", err)
	}
	if dbSvc == nil {
		t.Fatal("expected dbSvc not to be nil")
	}
	if filename != "compose.yml" {
		t.Errorf("filename = %q, want compose.yml", filename)
	}
	if dbSvc.Port != 5433 {
		t.Errorf("port = %d, want 5433 (long-syntax published)", dbSvc.Port)
	}
	if dbSvc.InternalPort != 5432 {
		t.Errorf("internal port = %d, want 5432", dbSvc.InternalPort)
	}
}

func TestDetectComposeUnquotedIntegerPort(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	content := `
services:
  db:
    image: postgres:16
    ports:
      - 5432
    environment:
      POSTGRES_DB: intport_dev
`
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yaml"), []byte(content), 0644); err != nil {
		t.Fatalf("failed to write compose file: %v", err)
	}

	dbSvc, _, err := DetectCompose(dir)
	if err != nil {
		t.Fatalf("DetectCompose failed: %v", err)
	}
	if dbSvc == nil {
		t.Fatal("expected dbSvc not to be nil")
	}
	if dbSvc.Port != 5432 {
		t.Errorf("port = %d, want 5432 (unquoted integer)", dbSvc.Port)
	}
}

func TestPortCollisionWarning(t *testing.T) {
	t.Parallel()
	got := PortCollisionWarning(5432, 5432, 5432)
	if !strings.Contains(got, "Port Collision Detected") {
		t.Fatalf("expected collision warning, got %q", got)
	}
	if !strings.Contains(got, "5433:5432") {
		t.Fatalf("expected remap suggestion 5433:5432, got %q", got)
	}
	if PortCollisionWarning(5433, 5432, 5432) != "" {
		t.Fatal("expected no warning when host port differs from proxy listen port")
	}
	if PortCollisionWarning(0, 5432, 5432) != "" {
		t.Fatal("expected no warning for missing host port")
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
