package compose

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestMariaDBCanonicalEnvironment(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	contents := `services:
  database:
    image: mariadb:11.4
    ports: ["3307:3306"]
    environment:
      MARIADB_USER: appuser
      MARIADB_PASSWORD: test-only-password
      MARIADB_DATABASE: appdb
`
	if err := os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
	got, _, err := DetectCompose(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("MariaDB service not detected")
	}
	if got.Driver != "mariadb" || got.User != "appuser" || got.Password != "test-only-password" || got.Database != "appdb" {
		t.Fatalf("MariaDB canonical variables ignored: driver=%q user=%q password_present=%v database=%q", got.Driver, got.User, got.Password != "", got.Database)
	}
}

func TestMariaDBFallbackAndPrecedence(t *testing.T) {
	t.Parallel()
	cases := []struct{ name, environment, user, password, database string }{
		{"legacy", "MYSQL_USER: legacyuser\n      MYSQL_PASSWORD: legacy-password\n      MYSQL_DATABASE: legacydb", "legacyuser", "legacy-password", "legacydb"},
		{"canonical root", "MARIADB_ROOT_PASSWORD: root-only-test-password", "root", "root-only-test-password", "myapp_dev"},
		{"legacy root", "MYSQL_ROOT_PASSWORD: legacy-root-password", "root", "legacy-root-password", "myapp_dev"},
		{"root precedence", "MARIADB_ROOT_PASSWORD: preferred-root-password\n      MYSQL_ROOT_PASSWORD: legacy-root-password", "root", "preferred-root-password", "myapp_dev"},
		{"canonical precedence", "MARIADB_USER: preferred\n      MYSQL_USER: legacy\n      MARIADB_PASSWORD: preferred-password\n      MYSQL_PASSWORD: legacy-password\n      MARIADB_DATABASE: preferred_db\n      MYSQL_DATABASE: legacy_db", "preferred", "preferred-password", "preferred_db"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			content := fmt.Sprintf("services:\n  db:\n    image: mariadb:11.4\n    environment:\n      %s\n", tc.environment)
			if err := os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte(content), 0600); err != nil {
				t.Fatal(err)
			}
			got, _, err := DetectCompose(dir)
			if err != nil {
				t.Fatal(err)
			}
			if got == nil || got.User != tc.user || got.Password != tc.password || got.Database != tc.database {
				t.Fatalf("incorrect MariaDB environment selection in %s", tc.name)
			}
		})
	}
}
