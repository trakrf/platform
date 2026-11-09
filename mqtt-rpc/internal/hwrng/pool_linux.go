//go:build linux

package hwrng

import (
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

// AddEntropy mixes data into the kernel's input pool and credits bits of
// entropy, which is what lets `crng init done` happen. RNDADDENTROPY comes from
// x/sys because its value differs by architecture (0x40085203 on arm and
// amd64, 0x80085203 on mips, ppc and sparc).
func (KernelPool) AddEntropy(data []byte, bits int) error {
	f, err := os.OpenFile("/dev/random", os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	info := poolInfo(data, bits)
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, f.Fd(), unix.RNDADDENTROPY, uintptr(unsafe.Pointer(&info[0])))
	if errno != 0 {
		return &os.SyscallError{Syscall: "ioctl RNDADDENTROPY", Err: errno}
	}
	return nil
}
