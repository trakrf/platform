//go:build !linux

package hwrng

import "errors"

// AddEntropy is Linux only; the daemon runs on a Linux reader.
func (KernelPool) AddEntropy([]byte, int) error {
	return errors.ErrUnsupported
}
