//go:build darwin

package sqlite

import (
	"syscall"
	"unsafe"
)

// CloneFile performs fast file snapshotting on macOS using the native APFS clonefile(2) syscall.
// If the underlying filesystem does not support clonefile (e.g. HFS+ or external non-APFS mounts),
// it falls back seamlessly to copyFileChunked.
func CloneFile(src, dst string) error {
	srcPtr, err := syscall.BytePtrFromString(src)
	if err != nil {
		return err
	}
	dstPtr, err := syscall.BytePtrFromString(dst)
	if err != nil {
		return err
	}

	// clonefile(src, dst, flags) is syscall 532 on Darwin / macOS
	_, _, errno := syscall.Syscall(syscall.SYS_CLONEFILE, uintptr(unsafe.Pointer(srcPtr)), uintptr(unsafe.Pointer(dstPtr)), 0)
	if errno != 0 {
		if errno == syscall.ENOTSUP || errno == syscall.ENOSYS || errno == syscall.EXDEV {
			return copyFileChunked(src, dst)
		}
		return errno
	}
	return nil
}
