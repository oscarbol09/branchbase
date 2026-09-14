package sqlite

import (
	"fmt"
	"io"
	"os"
)

// copyFileChunked provides a robust, cross-platform file copy using a 1MB buffer
// and preserves the source file's permission bits.
func copyFileChunked(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source file %q: %w", src, err)
	}
	defer func() {
		_ = srcFile.Close()
	}()

	info, err := srcFile.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat source file %q: %w", src, err)
	}

	dstFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
	if err != nil {
		return fmt.Errorf("failed to create destination file %q: %w", dst, err)
	}
	defer func() {
		_ = dstFile.Close()
	}()

	buf := make([]byte, 1024*1024) // 1MB buffer for high throughput
	if _, err := io.CopyBuffer(dstFile, srcFile, buf); err != nil {
		return fmt.Errorf("failed to copy data from %q to %q: %w", src, dst, err)
	}

	if err := dstFile.Sync(); err != nil {
		return fmt.Errorf("failed to sync destination file %q: %w", dst, err)
	}

	return nil
}
