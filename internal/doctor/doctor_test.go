package doctor

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/branchbase/branchbase/internal/driver"
)

type doctorMockDriver struct{}

func (d *doctorMockDriver) Name() string                        { return "mockdoc" }
func (d *doctorMockDriver) Ping(ctx context.Context) error      { return nil }
func (d *doctorMockDriver) Close() error                        { return nil }
func (d *doctorMockDriver) BranchExists(ctx context.Context, branchName string) (bool, error) {
	return true, nil
}
func (d *doctorMockDriver) CreateBranch(ctx context.Context, sourceBranch, targetBranch string) error {
	return nil
}
func (d *doctorMockDriver) DeleteBranch(ctx context.Context, branchName string) error {
	return nil
}
func (d *doctorMockDriver) ListBranches(ctx context.Context) ([]driver.BranchInfo, error) {
	return nil, nil
}

func init() {
	driver.Register("mockdoc", func(params map[string]interface{}) (driver.Driver, error) {
		return &doctorMockDriver{}, nil
	})
}

func TestDoctorRun(t *testing.T) {
	tempDir := t.TempDir()
	gitDir := filepath.Join(tempDir, ".git")
	_ = os.MkdirAll(gitDir, 0755)
	_ = os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/main\n"), 0644)

	cfgPath := filepath.Join(tempDir, ".branchbase.json")
	cfgContent := `{
		"driver": "mockdoc",
		"connection": {
			"host": "127.0.0.1",
			"port": 5433,
			"base_database": "myapp_dev"
		},
		"proxy": {
			"listen_port": 64321,
			"default_branch": "main"
		}
	}`
	_ = os.WriteFile(cfgPath, []byte(cfgContent), 0644)

	ctx := context.Background()
	report := Run(ctx, tempDir, cfgPath)

	if len(report.Checks) == 0 {
		t.Fatalf("expected checks in doctor report, got 0")
	}

	formatted := FormatReport(report)
	if !strings.Contains(formatted, "BranchBase Doctor") {
		t.Errorf("expected formatted report to contain header, got:\n%s", formatted)
	}
	if !strings.Contains(formatted, "Git Repository") {
		t.Errorf("expected Git Repository check, got:\n%s", formatted)
	}
	if !strings.Contains(formatted, "Config") {
		t.Errorf("expected Config check, got:\n%s", formatted)
	}
}
