package hwrng

import "encoding/binary"

// rndAddEntropy is RNDADDENTROPY, _IOW('R', 0x03, int[2]) in <linux/random.h>.
const rndAddEntropy = 0x40085203

// poolInfo encodes struct rand_pool_info: int entropy_count (bits),
// int buf_size (bytes), then the bytes themselves.
func poolInfo(data []byte, bits int) []byte {
	out := make([]byte, 8+len(data))
	binary.NativeEndian.PutUint32(out[0:4], uint32(int32(bits)))
	binary.NativeEndian.PutUint32(out[4:8], uint32(int32(len(data))))
	copy(out[8:], data)
	return out
}

// KernelPool credits entropy to the kernel through /dev/random. It needs
// CAP_SYS_ADMIN; mqtt-rpcd runs as root.
type KernelPool struct {
	// Path defaults to /dev/random.
	Path string
}

func (k KernelPool) path() string {
	if k.Path == "" {
		return "/dev/random"
	}
	return k.Path
}
