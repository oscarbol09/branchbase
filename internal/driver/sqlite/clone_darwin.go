//go:build darwin

package sqlite

import (
	"syscall"
	"unsafe"
)

// sysClonefile is the Darwin / macOS syscall number for clonefile(2) (sys/syscall.h SYS_clonefile = 532)
const sysClonefile = 532

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
	_, _, errno := syscall.Syscall(sysClonefile, uintptr(unsafe.Pointer(srcPtr)), uintptr(unsafe.Pointer(dstPtr)), 0)
	if errno != 0 {
		return copyFileChunked(src, dst)
	}
	return nil
}
