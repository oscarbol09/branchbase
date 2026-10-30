package sqlite

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestExtensionlessCopyMarkerNotListed(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"app", "app_ready", "app_pending.branchbase-copying"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("test"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	drv, err := New(Config{BasePath: filepath.Join(dir, "app")})
	if err != nil {
		t.Fatal(err)
	}
	got, err := drv.ListBranches(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("copy reservation was exposed as a database branch: %#v", got)
	}
}

func TestCopyMarkersWithUnusualExtensions(t *testing.T) {
	for _, base := range []string{"app.db", "app.branchbase-copying", "app.extra.branchbase-copying"} {
		t.Run(base, func(t *testing.T) {
			dir := t.TempDir()
			drv, err := New(Config{BasePath: filepath.Join(dir, base)})
			if err != nil {
				t.Fatal(err)
			}
			for _, filename := range []string{drv.formatDBPath("main"), drv.formatDBPath("ready"), drv.formatDBPath("pending") + ".branchbase-copying"} {
				if err := os.WriteFile(filename, []byte("test"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			got, err := drv.ListBranches(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != 2 {
				t.Fatalf("got %d branches, want real main and ready branches", len(got))
			}
		})
	}
}
