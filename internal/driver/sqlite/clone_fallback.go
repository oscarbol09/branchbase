//go:build !linux && !darwin

package sqlite

// CloneFile performs high-throughput chunked file copying on Windows and other platforms.
func CloneFile(src, dst string) error {
	return copyFileChunked(src, dst)
}
