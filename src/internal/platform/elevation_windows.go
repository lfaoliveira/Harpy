//go:build windows

package platform

import (
	"syscall"
	"unsafe"
)

// IsElevated reports whether the process token is elevated (run as
// Administrator).
func IsElevated() bool {
	token, err := syscall.OpenCurrentProcessToken()
	if err != nil {
		return false
	}
	defer token.Close()

	var elevated uint32
	var returned uint32
	err = syscall.GetTokenInformation(
		token,
		syscall.TokenElevation,
		(*byte)(unsafe.Pointer(&elevated)),
		uint32(unsafe.Sizeof(elevated)),
		&returned,
	)
	return err == nil && elevated != 0
}
