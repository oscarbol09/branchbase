package driver

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// BranchInfo contains metadata about an ephemeral database branch
type BranchInfo struct {
	Name        string    `json:"name"`
	Database    string    `json:"database"`
	CreatedAt   time.Time `json:"created_at"`
	SizeBytes   int64     `json:"size_bytes"`
	IsActive    bool      `json:"is_active"`
	IsProtected bool      `json:"is_protected"`
}

// Driver defines the behavior required for database engines supported by BranchBase
type Driver interface {
	// Name returns the driver identifier (e.g. "postgres", "sqlite")
	Name() string

	// Ping checks connectivity with the underlying database server
	Ping(ctx context.Context) error

	// BranchExists checks whether an isolated database for this branch exists
	BranchExists(ctx context.Context, branchName string) (bool, error)

	// CreateBranch clones sourceBranch into targetBranch
	CreateBranch(ctx context.Context, sourceBranch, targetBranch string) error

	// DeleteBranch destroys an ephemeral database branch
	DeleteBranch(ctx context.Context, branchName string) error

	// ListBranches returns all databases currently managed by BranchBase
	ListBranches(ctx context.Context) ([]BranchInfo, error)
}

var (
	registryMu sync.RWMutex
	registry   = make(map[string]DriverFactory)
)

type DriverFactory func(params map[string]interface{}) (Driver, error)

// Register registers a database driver factory
func Register(name string, factory DriverFactory) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[name] = factory
}

// GetDriver returns an instance of the registered driver
func GetDriver(name string, params map[string]interface{}) (Driver, error) {
	registryMu.RLock()
	factory, exists := registry[name]
	registryMu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("driver %q is not registered or supported yet", name)
	}

	return factory(params)
}
