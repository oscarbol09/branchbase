//go:build linux

package sqlite

import (
	"fmt"
	"os"
	"syscall"
)

// ficlone is the ioctl code for Linux filesystem Copy-on-Write (reflink) cloning.
// Defined in linux/fs.h: _IOW(0x94, 9, int) = 0x40049409
const ficlone = 0x40049409

// CloneFile attempts an instantaneous Copy-on-Write (reflink) clone via ioctl FICLONE
// on supported filesystems (Btrfs, XFS, ZFS, OCFS2). If the filesystem does not support
// reflinks (e.g. ext4, or cross-filesystem copy), it gracefully falls back to fast
// chunked file copy.
func CloneFile(src, dst string) error {
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

	// Attempt Linux FICLONE ioctl
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, dstFile.Fd(), ficlone, srcFile.Fd())
	_ = dstFile.Close()

	if errno == 0 {
		// Reflink succeeded!
		return nil
	}

	// Fallback to streaming chunked copy if FICLONE is unsupported on this filesystem
	return copyFileChunked(src, dst)
}
