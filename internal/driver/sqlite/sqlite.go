package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/branchbase/branchbase/internal/driver"
	"github.com/branchbase/branchbase/internal/git"
	_ "modernc.org/sqlite"
)

// Config holds connection and path parameters for SQLite
type Config struct {
	BasePath      string // Path to primary database file (e.g. "myapp_dev.db" or "./data/app.db")
	DefaultBranch string
}

// SqliteDriver manages filesystem-level database branching for SQLite engines
type SqliteDriver struct {
	cfg Config
	mu  sync.RWMutex
}

func init() {
	driver.Register("sqlite", func(params map[string]interface{}) (driver.Driver, error) {
		cfg := Config{
			BasePath: "myapp_dev.db",
		}

		if p, ok := params["path"].(string); ok && p != "" {
			cfg.BasePath = p
		}
		if base, ok := params["base_database"].(string); ok && base != "" {
			if cfg.BasePath == "myapp_dev.db" {
				cfg.BasePath = base + ".db"
			}
		}
		if def, ok := params["default_branch"].(string); ok && strings.TrimSpace(def) != "" {
			cfg.DefaultBranch = def
		}

		return New(cfg)
	})
}

// New creates a new SqliteDriver instance
func New(cfg Config) (*SqliteDriver, error) {
	if cfg.BasePath == "" {
		return nil, errors.New("sqlite: base_path cannot be empty")
	}
	return &SqliteDriver{cfg: cfg}, nil
}

func (d *SqliteDriver) Name() string {
	return "sqlite"
}

// Ping checks whether the SQLite database file or its parent directory is accessible
func (d *SqliteDriver) Ping(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	d.mu.RLock()
	basePath := d.cfg.BasePath
	d.mu.RUnlock()

	dir := filepath.Dir(basePath)
	if dir != "" && dir != "." {
		if dirInfo, err := os.Stat(dir); err != nil {
			return fmt.Errorf("sqlite base directory %q inaccessible: %w", dir, err)
		} else if !dirInfo.IsDir() {
			return fmt.Errorf("sqlite base directory %q is not a directory", dir)
		}
	}

	if info, err := os.Stat(basePath); err == nil {
		if info.IsDir() {
			return fmt.Errorf("sqlite base path %q is a directory, expected database file", basePath)
		}
		// Verify readable
		f, err := os.Open(basePath)
		if err != nil {
			return fmt.Errorf("sqlite database file %q unreadable: %w", basePath, err)
		}
		if err := f.Close(); err != nil { fmt.Fprintf(os.Stderr, "failed to close f: %v\n", err) }
	}

	return nil
}

// BranchExists checks whether a database file exists for the given branch
func (d *SqliteDriver) BranchExists(ctx context.Context, branchName string) (bool, error) {
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	default:
	}

	targetPath := d.formatDBPath(branchName)
	if _, err := os.Stat(targetPath + ".branchbase-copying"); err == nil {
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, fmt.Errorf("failed to inspect SQLite copy marker for %q: %w", targetPath, err)
	}
	info, err := os.Stat(targetPath)
	if err == nil {
		return !info.IsDir(), nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// CreateBranch clones sourceBranch into targetBranch using Copy-on-Write / fast streaming
func (d *SqliteDriver) CreateBranch(ctx context.Context, sourceBranch, targetBranch string) (retErr error) {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	sourcePath := d.formatDBPath(sourceBranch)
	targetPath := d.formatDBPath(targetBranch)

	if sourcePath == targetPath {
		return fmt.Errorf("source and target branch database paths are identical: %q", sourcePath)
	}

	srcInfo, err := os.Stat(sourcePath)
	if err != nil {
		return fmt.Errorf("cannot create branch: source database %q does not exist: %w", sourcePath, err)
	}
	if srcInfo.IsDir() {
		return fmt.Errorf("source database %q is a directory", sourcePath)
	}
	if err := checkpointSQLite(ctx, sourcePath); err != nil {
		return err
	}

	// Ensure destination directory exists
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return fmt.Errorf("failed to create destination directory for %q: %w", targetPath, err)
	}

	for _, path := range []string{targetPath, targetPath + "-wal", targetPath + "-shm", targetPath + "-journal"} {
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("refusing to overwrite existing SQLite branch artifact %q", path)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("failed to inspect SQLite branch artifact %q: %w", path, err)
		}
	}
	copyMarker := targetPath + ".branchbase-copying"
	marker, err := os.OpenFile(copyMarker, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("failed to reserve SQLite branch copy %q: %w", targetPath, err)
	}
	if err := marker.Close(); err != nil {
		cleanupErr := os.Remove(copyMarker)
		return errors.Join(fmt.Errorf("failed to close SQLite copy marker %q: %w", copyMarker, err), cleanupErr)
	}
	defer func() {
		if err := os.Remove(copyMarker); err != nil && !os.IsNotExist(err) {
			retErr = errors.Join(retErr, fmt.Errorf("failed to remove SQLite copy marker %q: %w", copyMarker, err))
		}
	}()

	reservation, err := os.OpenFile(targetPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, srcInfo.Mode().Perm())
	if err != nil {
		return fmt.Errorf("failed to reserve SQLite branch path %q: %w", targetPath, err)
	}
	if err := reservation.Close(); err != nil {
		cleanupErr := os.Remove(targetPath)
		return errors.Join(fmt.Errorf("failed to close SQLite branch reservation %q: %w", targetPath, err), cleanupErr)
	}

	if err := CloneFile(sourcePath, targetPath); err != nil {
		cleanupErr := os.Remove(targetPath)
		return errors.Join(fmt.Errorf("failed to snapshot sqlite database from %q to %q: %w", sourcePath, targetPath, err), cleanupErr)
	}
	finalInfo, err := os.Stat(sourcePath)
	if err != nil {
		cleanupErr := os.Remove(targetPath)
		return errors.Join(fmt.Errorf("failed to verify source database after snapshot: %w", err), cleanupErr)
	}
	if srcInfo.Size() != finalInfo.Size() || !srcInfo.ModTime().Equal(finalInfo.ModTime()) {
		cleanupErr := os.Remove(targetPath)
		return errors.Join(fmt.Errorf("source SQLite database changed during snapshot; retry after writes stop"), cleanupErr)
	}
	if err := checkpointSQLite(ctx, sourcePath); err != nil {
		cleanupErr := os.Remove(targetPath)
		return errors.Join(err, cleanupErr)
	}

	return nil
}

func checkpointSQLite(ctx context.Context, databasePath string) error {
	db, err := sql.Open("sqlite", databasePath)
	if err != nil {
		return fmt.Errorf("failed to open database to checkpoint: %w", err)
	}
	defer func() {
		if err := db.Close(); err != nil { fmt.Fprintf(os.Stderr, "failed to close db: %v\n", err) } // Best effort close
	}()

	// Execute TRUNCATE checkpoint
	_, err = db.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE);")
	if err != nil {
		if strings.Contains(err.Error(), "file is not a database") || strings.Contains(err.Error(), "not an error") {
			// Ignore for test dummy files or uninitialized DBs
		} else {
			return fmt.Errorf("failed to execute PRAGMA wal_checkpoint: %w", err)
		}
	}

	// Double check sidecars are gone
	for _, suffix := range []string{"-wal", "-shm"} {
		path := databasePath + suffix
		if info, err := os.Stat(path); err == nil && info.Size() > 0 {
			return fmt.Errorf("failed to truncate SQLite sidecar %q", path)
		} else if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to inspect SQLite sidecar %q: %w", path, err)
		}
	}
	return nil
}

// DeleteBranch destroys an ephemeral SQLite database branch along with WAL/SHM artifacts
func (d *SqliteDriver) DeleteBranch(ctx context.Context, branchName string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	sanitized := git.SanitizeBranchName(branchName)
	if d.isBaseBranch(sanitized) {
		return fmt.Errorf("cannot delete protected base database %q", d.cfg.BasePath)
	}

	targetPath := d.formatDBPath(branchName)
	if _, err := os.Stat(targetPath + ".branchbase-copying"); err == nil {
		return fmt.Errorf("cannot delete SQLite branch %q while a copy is in progress", targetPath)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to inspect SQLite copy marker for %q: %w", targetPath, err)
	}
	if targetPath == d.cfg.BasePath {
		return fmt.Errorf("cannot delete protected base database %q", d.cfg.BasePath)
	}

	if err := os.Remove(targetPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete branch database %q: %w", targetPath, err)
	}
	_ = os.Remove(targetPath + "-wal")
	_ = os.Remove(targetPath + "-shm")

	return nil
}

// ListBranches returns all SQLite database files matching the base filename prefix in the same directory
func (d *SqliteDriver) ListBranches(ctx context.Context) ([]driver.BranchInfo, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	d.mu.RLock()
	basePath := d.cfg.BasePath
	d.mu.RUnlock()

	dir := filepath.Dir(basePath)
	if dir == "" {
		dir = "."
	}
	baseFile := filepath.Base(basePath)
	ext := filepath.Ext(baseFile)
	basePrefix := strings.TrimSuffix(baseFile, ext)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read sqlite directory %q: %w", dir, err)
	}

	var branches []driver.BranchInfo
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if _, err := os.Stat(filepath.Join(dir, name+".branchbase-copying")); err == nil {
			continue
		}
		// Exclude SQLite journal, WAL, and SHM sidecar files
		if strings.HasSuffix(name, "-wal") || strings.HasSuffix(name, "-shm") || strings.HasSuffix(name, "-journal") {
			continue
		}

		var branchName string
		var isProtected bool

		if name == baseFile {
			branchName = d.listedDefaultBranch()
			isProtected = true
		} else if strings.HasPrefix(name, basePrefix+"_") && strings.HasSuffix(name, ext) {
			branchName = strings.TrimPrefix(name, basePrefix+"_")
			branchName = strings.TrimSuffix(branchName, ext)
			isProtected = false
		} else {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		size := info.Size()
		// Include WAL file size if it exists
		if walInfo, err := os.Stat(filepath.Join(dir, name+"-wal")); err == nil {
			size += walInfo.Size()
		}

		branches = append(branches, driver.BranchInfo{
			Name:        branchName,
			Database:    name,
			CreatedAt:   info.ModTime(),
			SizeBytes:   size,
			IsActive:    false,
			IsProtected: isProtected,
		})
	}

	return branches, nil
}

// Close releases any held resources for the SQLite driver (no-op for file-based SQLite)
func (d *SqliteDriver) Close() error {
	return nil
}

// formatDBPath converts a branch name into the corresponding SQLite database file path
func (d *SqliteDriver) formatDBPath(branch string) string {
	sanitized := git.SanitizeBranchName(branch)
	if d.isBaseBranch(sanitized) {
		return d.cfg.BasePath
	}

	dir := filepath.Dir(d.cfg.BasePath)
	baseFile := filepath.Base(d.cfg.BasePath)
	ext := filepath.Ext(baseFile)
	basePrefix := strings.TrimSuffix(baseFile, ext)

	targetFile := fmt.Sprintf("%s_%s%s", basePrefix, sanitized, ext)
	if dir == "" || dir == "." {
		return targetFile
	}
	return filepath.Join(dir, targetFile)
}

func (d *SqliteDriver) isBaseBranch(sanitized string) bool {
	def := strings.TrimSpace(d.cfg.DefaultBranch)
	if def == "" {
		return sanitized == "main" || sanitized == "master" || sanitized == "default"
	}
	return sanitized == git.SanitizeBranchName(def)
}

func (d *SqliteDriver) listedDefaultBranch() string {
	def := strings.TrimSpace(d.cfg.DefaultBranch)
	if def == "" {
		return "main"
	}
	return git.SanitizeBranchName(def)
}
