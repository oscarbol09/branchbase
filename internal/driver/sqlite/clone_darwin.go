//go:build darwin

package sqlite

// CloneFile performs fast file snapshotting on macOS.
// Utilizes high-throughput chunked file copying with sync.
func CloneFile(src, dst string) error {
	return copyFileChunked(src, dst)
}
