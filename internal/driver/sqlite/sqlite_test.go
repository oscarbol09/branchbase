package sqlite

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/branchbase/branchbase/internal/driver"
)

func TestDriverRegistration(t *testing.T) {
	drv, err := driver.GetDriver("sqlite", map[string]interface{}{
		"path": "test_app.db",
	})
	if err != nil {
		t.Fatalf("driver.GetDriver('sqlite') failed: %v", err)
	}
	if drv.Name() != "sqlite" {
		t.Fatalf("drv.Name() = %q, want 'sqlite'", drv.Name())
	}
}

func TestPing(t *testing.T) {
	tempDir := t.TempDir()
	basePath := filepath.Join(tempDir, "app.db")

	drv, err := New(Config{BasePath: basePath})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	ctx := context.Background()
	// Ping when file does not exist yet (parent dir exists and is writable)
	if err := drv.Ping(ctx); err != nil {
		t.Fatalf("Ping failed before file creation: %v", err)
	}

	// Create dummy file
	if err := os.WriteFile(basePath, []byte("SQLite format 3\x00dummy data"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Ping when file exists and is readable
	if err := drv.Ping(ctx); err != nil {
		t.Fatalf("Ping failed after file creation: %v", err)
	}
}

func TestCreateBranchAndExists(t *testing.T) {
	tempDir := t.TempDir()
	basePath := filepath.Join(tempDir, "myapp_dev.db")

	drv, err := New(Config{BasePath: basePath})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	ctx := context.Background()

	// Source does not exist yet -> CreateBranch should fail
	if err := drv.CreateBranch(ctx, "main", "feature/auth"); err == nil {
		t.Fatal("expected error creating branch from non-existent source, got nil")
	}

	// Create source base file
	sourceContent := []byte("sqlite-header-test-data-block-12345")
	if err := os.WriteFile(basePath, sourceContent, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Check branch existence before cloning
	exists, err := drv.BranchExists(ctx, "feature/auth")
	if err != nil {
		t.Fatalf("BranchExists: %v", err)
	}
	if exists {
		t.Fatal("expected feature/auth to not exist before clone")
	}

	// Clone from main into feature/auth
	if err := drv.CreateBranch(ctx, "main", "feature/auth"); err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}

	// Check branch existence after cloning
	exists, err = drv.BranchExists(ctx, "feature/auth")
	if err != nil {
		t.Fatalf("BranchExists: %v", err)
	}
	if !exists {
		t.Fatal("expected feature/auth to exist after clone")
	}

	// Target file name must be sanitized: feature/auth -> myapp_dev_feature_auth.db
	targetPath := filepath.Join(tempDir, "myapp_dev_feature_auth.db")
	clonedContent, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("ReadFile(targetPath): %v", err)
	}
	if string(clonedContent) != string(sourceContent) {
		t.Fatalf("cloned content = %q, want %q", string(clonedContent), string(sourceContent))
	}
}

func TestDataIsolation(t *testing.T) {
	tempDir := t.TempDir()
	basePath := filepath.Join(tempDir, "myapp_dev.db")

	drv, err := New(Config{BasePath: basePath})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	ctx := context.Background()
	initialContent := []byte("original-unmutated-main-database")
	if err := os.WriteFile(basePath, initialContent, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Create branch
	if err := drv.CreateBranch(ctx, "main", "feature/payments"); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}

	// Mutate branch database
	branchPath := filepath.Join(tempDir, "myapp_dev_feature_payments.db")
	mutatedContent := []byte("mutated-payments-branch-data-modified-rows")
	if err := os.WriteFile(branchPath, mutatedContent, 0o644); err != nil {
		t.Fatalf("WriteFile branch: %v", err)
	}

	// Assert source database was NOT modified
	sourceAfter, err := os.ReadFile(basePath)
	if err != nil {
		t.Fatalf("ReadFile source: %v", err)
	}
	if string(sourceAfter) != string(initialContent) {
		t.Fatalf("source database mutated! got %q, want %q", string(sourceAfter), string(initialContent))
	}
}

func TestListBranches(t *testing.T) {
	tempDir := t.TempDir()
	basePath := filepath.Join(tempDir, "myapp_dev.db")

	drv, err := New(Config{BasePath: basePath})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	ctx := context.Background()
	_ = os.WriteFile(basePath, []byte("main-db-data"), 0o644)
	_ = os.WriteFile(filepath.Join(tempDir, "myapp_dev_feat1.db"), []byte("feat1-db-data"), 0o644)
	_ = os.WriteFile(filepath.Join(tempDir, "myapp_dev_feat2.db"), []byte("feat2-db-data"), 0o644)
	// Sidecar and unrelated files should be ignored
	_ = os.WriteFile(filepath.Join(tempDir, "myapp_dev_feat1.db-wal"), []byte("wal"), 0o644)
	_ = os.WriteFile(filepath.Join(tempDir, "unrelated.txt"), []byte("ignore"), 0o644)

	branches, err := drv.ListBranches(ctx)
	if err != nil {
		t.Fatalf("ListBranches failed: %v", err)
	}

	if len(branches) != 3 {
		t.Fatalf("got %d branches, want 3", len(branches))
	}

	branchMap := make(map[string]driver.BranchInfo)
	for _, b := range branches {
		branchMap[b.Name] = b
	}

	mainBranch, ok := branchMap["main"]
	if !ok {
		t.Fatal("missing 'main' branch in list")
	}
	if !mainBranch.IsProtected {
		t.Fatal("expected 'main' branch to be marked as protected")
	}

	feat1, ok := branchMap["feat1"]
	if !ok {
		t.Fatal("missing 'feat1' branch in list")
	}
	if feat1.IsProtected {
		t.Fatal("expected 'feat1' branch to NOT be protected")
	}
}

func TestDeleteBranchProtection(t *testing.T) {
	tempDir := t.TempDir()
	basePath := filepath.Join(tempDir, "myapp_dev.db")

	drv, err := New(Config{BasePath: basePath})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	ctx := context.Background()
	_ = os.WriteFile(basePath, []byte("protected-data"), 0o644)

	// Attempting to delete main must fail with protection error
	for _, protected := range []string{"main", "master", "default", ""} {
		if err := drv.DeleteBranch(ctx, protected); err == nil {
			t.Fatalf("expected error deleting protected branch %q, got nil", protected)
		}
	}

	// Base file must still exist
	if _, err := os.Stat(basePath); err != nil {
		t.Fatalf("base database was deleted: %v", err)
	}

	// Create and then delete ephemeral branch
	if err := drv.CreateBranch(ctx, "main", "temp-branch"); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	branchPath := filepath.Join(tempDir, "myapp_dev_temp_branch.db")
	if _, err := os.Stat(branchPath); err != nil {
		t.Fatalf("branch file does not exist before delete: %v", err)
	}

	if err := drv.DeleteBranch(ctx, "temp-branch"); err != nil {
		t.Fatalf("DeleteBranch failed: %v", err)
	}

	if _, err := os.Stat(branchPath); !os.IsNotExist(err) {
		t.Fatalf("expected branch file to be deleted, err: %v", err)
	}
}

func TestCloneWithWALAndSHM(t *testing.T) {
	tempDir := t.TempDir()
	basePath := filepath.Join(tempDir, "myapp_dev.db")

	drv, err := New(Config{BasePath: basePath})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	ctx := context.Background()
	_ = os.WriteFile(basePath, []byte("db"), 0o644)
	_ = os.WriteFile(basePath+"-wal", []byte("wal-bytes"), 0o644)
	_ = os.WriteFile(basePath+"-shm", []byte("shm-bytes"), 0o644)

	if err := drv.CreateBranch(ctx, "main", "wal-test"); err != nil {
		t.Fatalf("CreateBranch with WAL: %v", err)
	}

	targetDB := filepath.Join(tempDir, "myapp_dev_wal_test.db")
	if _, err := os.Stat(targetDB); err != nil {
		t.Fatalf("target DB not found: %v", err)
	}
	if _, err := os.Stat(targetDB + "-wal"); err != nil {
		t.Fatalf("target WAL not cloned: %v", err)
	}
	if _, err := os.Stat(targetDB + "-shm"); err != nil {
		t.Fatalf("target SHM not cloned: %v", err)
	}

	// Clean up branch
	if err := drv.DeleteBranch(ctx, "wal-test"); err != nil {
		t.Fatalf("DeleteBranch: %v", err)
	}
	if _, err := os.Stat(targetDB + "-wal"); !os.IsNotExist(err) {
		t.Fatal("target WAL was not cleaned up on DeleteBranch")
	}
}
