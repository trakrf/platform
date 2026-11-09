//go:build linux

package hwrng

import (
	"os"
	"syscall"
	"unsafe"
)

// AddEntropy mixes data into the kernel's input pool and credits bits of
// entropy, which is what lets `crng init done` happen.
func (k KernelPool) AddEntropy(data []byte, bits int) error {
	f, err := os.OpenFile(k.path(), os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	info := poolInfo(data, bits)
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), rndAddEntropy, uintptr(unsafe.Pointer(&info[0])))
	if errno != 0 {
		return &os.SyscallError{Syscall: "ioctl RNDADDENTROPY", Err: errno}
	}
	return nil
}
