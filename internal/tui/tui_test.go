package tui

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/branchbase/branchbase/internal/config"
	"github.com/branchbase/branchbase/internal/driver"
)

type mockDriver struct {
	branches    []driver.BranchInfo
	created     map[string]bool
	listErr     error
	existsErr   error
	createErr   error
}

func (m *mockDriver) Name() string { return "mock" }
func (m *mockDriver) Ping(ctx context.Context) error { return nil }
func (m *mockDriver) Close() error { return nil }

func (m *mockDriver) BranchExists(ctx context.Context, branchName string) (bool, error) {
	if m.existsErr != nil {
		return false, m.existsErr
	}
	if m.created != nil && m.created[branchName] {
		return true, nil
	}
	for _, b := range m.branches {
		if b.Name == branchName {
			return true, nil
		}
	}
	return false, nil
}

func (m *mockDriver) CreateBranch(ctx context.Context, sourceBranch, targetBranch string) error {
	if m.createErr != nil {
		return m.createErr
	}
	if m.created == nil {
		m.created = make(map[string]bool)
	}
	m.created[targetBranch] = true
	m.branches = append(m.branches, driver.BranchInfo{
		Name:     targetBranch,
		Database: "myapp_dev_" + targetBranch,
	})
	return nil
}

func (m *mockDriver) DeleteBranch(ctx context.Context, branchName string) error {
	return nil
}

func (m *mockDriver) ListBranches(ctx context.Context) ([]driver.BranchInfo, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.branches, nil
}

func TestTUIModelNavigationAndClamping(t *testing.T) {
	t.Parallel()

	drv := &mockDriver{
		branches: []driver.BranchInfo{
			{Name: "main", Database: "myapp_dev", SizeBytes: 1024 * 1024, IsProtected: true},
			{Name: "feature_a", Database: "myapp_dev_feature_a", SizeBytes: 2 * 1024 * 1024},
			{Name: "feature_b", Database: "myapp_dev_feature_b", SizeBytes: 3 * 1024 * 1024},
		},
	}

	model := NewModel(t.TempDir(), nil, drv)
	ctx := context.Background()
	if err := model.Refresh(ctx); err != nil {
		t.Fatalf("Refresh failed: %v", err)
	}

	if model.SelectedIndex != 0 {
		t.Fatalf("initial index = %d, want 0", model.SelectedIndex)
	}

	// Move down
	model.MoveDown()
	if model.SelectedIndex != 1 {
		t.Fatalf("after MoveDown index = %d, want 1", model.SelectedIndex)
	}

	model.MoveDown()
	if model.SelectedIndex != 2 {
		t.Fatalf("after MoveDown index = %d, want 2", model.SelectedIndex)
	}

	// Move down beyond last item (clamping)
	model.MoveDown()
	if model.SelectedIndex != 2 {
		t.Fatalf("clamped MoveDown index = %d, want 2", model.SelectedIndex)
	}

	// Move up
	model.MoveUp()
	if model.SelectedIndex != 1 {
		t.Fatalf("after MoveUp index = %d, want 1", model.SelectedIndex)
	}

	model.MoveUp()
	if model.SelectedIndex != 0 {
		t.Fatalf("after MoveUp index = %d, want 0", model.SelectedIndex)
	}

	// Move up beyond 0 (clamping)
	model.MoveUp()
	if model.SelectedIndex != 0 {
		t.Fatalf("clamped MoveUp index = %d, want 0", model.SelectedIndex)
	}
}

func TestTUIModelSelectedBranch(t *testing.T) {
	t.Parallel()

	modelEmpty := NewModel(t.TempDir(), nil, nil)
	if sel := modelEmpty.SelectedBranch(); sel != nil {
		t.Fatalf("expected nil for empty model, got %+v", sel)
	}

	drv := &mockDriver{
		branches: []driver.BranchInfo{
			{Name: "main", Database: "myapp_dev"},
			{Name: "feat_x", Database: "myapp_dev_feat_x"},
		},
	}

	model := NewModel(t.TempDir(), nil, drv)
	_ = model.Refresh(context.Background())

	if sel := model.SelectedBranch(); sel == nil || sel.Name != "main" {
		t.Fatalf("expected main, got %+v", sel)
	}

	model.MoveDown()
	if sel := model.SelectedBranch(); sel == nil || sel.Name != "feat_x" {
		t.Fatalf("expected feat_x, got %+v", sel)
	}
}

func TestTUIRenderView(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	gitDir := filepath.Join(tempDir, ".git")
	_ = os.MkdirAll(gitDir, 0755)
	_ = os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/feature_ui_test\n"), 0644)

	cfg := config.DefaultConfig()
	cfg.Connection.Host = "127.0.0.1"
	cfg.Connection.Port = 5433
	cfg.Proxy.ListenPort = 5432

	drv := &mockDriver{
		branches: []driver.BranchInfo{
			{Name: "main", Database: "myapp_dev", SizeBytes: 1048576, IsProtected: true},
			{Name: "feature_ui_test", Database: "myapp_dev_feature_ui_test", SizeBytes: 2097152},
		},
	}

	model := NewModel(tempDir, &cfg, drv)
	ctx := context.Background()
	_ = model.Refresh(ctx)

	view := model.RenderView()
	if !strings.Contains(view, "BranchBase Dashboard") {
		t.Errorf("missing header in RenderView:\n%s", view)
	}
	if !strings.Contains(view, "feature_ui_test") {
		t.Errorf("missing active branch in RenderView:\n%s", view)
	}
	if !strings.Contains(view, "myapp_dev_feature_ui_test") {
		t.Errorf("missing branch database in table:\n%s", view)
	}
	if !strings.Contains(view, "Active") {
		t.Errorf("missing Active status indicator:\n%s", view)
	}
	if !strings.Contains(view, "Protected") {
		t.Errorf("missing Protected status indicator:\n%s", view)
	}
}

func TestTUIHandleKeyActions(t *testing.T) {
	t.Parallel()

	drv := &mockDriver{
		branches: []driver.BranchInfo{
			{Name: "main", Database: "myapp_dev", IsProtected: true},
			{Name: "feature_unprovisioned", Database: "myapp_dev_feature_unprovisioned"},
		},
	}

	model := NewModel(t.TempDir(), nil, drv)
	ctx := context.Background()
	_ = model.Refresh(ctx)

	// Test 'q' key
	quit, _ := model.HandleKey("q", ctx)
	if !quit || !model.IsQuitting {
		t.Errorf("expected q to quit, got quit=%v isQuitting=%v", quit, model.IsQuitting)
	}

	// Test 'j' key (down)
	quit, _ = model.HandleKey("j", ctx)
	if quit || model.SelectedIndex != 1 {
		t.Errorf("expected j to move down to index 1, got index=%d", model.SelectedIndex)
	}

	// Test 'k' key (up)
	quit, _ = model.HandleKey("k", ctx)
	if quit || model.SelectedIndex != 0 {
		t.Errorf("expected k to move up to index 0, got index=%d", model.SelectedIndex)
	}

	// Test 'r' key (refresh)
	quit, err := model.HandleKey("r", ctx)
	if quit || err != nil || model.FlashType != "success" {
		t.Errorf("expected refresh success, got err=%v flashType=%s", err, model.FlashType)
	}

	// Test 'enter' key on unprovisioned branch
	model.MoveDown() // select feature_unprovisioned
	quit, err = model.HandleKey("enter", ctx)
	if quit || err != nil {
		t.Fatalf("unexpected error on enter: %v", err)
	}
	if model.FlashType != "success" {
		t.Errorf("expected flashType success on branch switch, got %s", model.FlashType)
	}
}

func TestTUIRunEventLoop(t *testing.T) {
	t.Parallel()

	drv := &mockDriver{
		branches: []driver.BranchInfo{
			{Name: "main", Database: "myapp_dev"},
		},
	}

	cfg := config.DefaultConfig()
	input := bytes.NewBufferString("j\nr\nq")
	var output bytes.Buffer

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := Run(ctx, t.TempDir(), &cfg, drv, input, &output)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	outStr := output.String()
	if !strings.Contains(outStr, "BranchBase Dashboard") {
		t.Fatalf("expected output to contain dashboard render, got:\n%s", outStr)
	}
}
