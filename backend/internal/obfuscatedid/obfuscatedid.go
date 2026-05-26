// Package obfuscatedid implements the keyed Feistel ID generator used by
// trakrf.generate_obfuscated_id() in the database. This Go implementation is
// the reference oracle for test vectors and the PL/pgSQL parity check.
//
// Construction: 52-bit Feistel (2 x 26-bit halves), 6 rounds, HMAC-SHA256 round
// function truncated to 26 bits. Output range: [0, 2^52).
package obfuscatedid

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
)

const (
	blockBits = 52
	halfBits  = 26
	rounds    = 6
	mask26    = (uint64(1) << halfBits) - 1
)

// Encrypt maps a sequence value into a 52-bit obfuscated ID. seqValue must be
// less than 2^52 (the Feistel block size).
func Encrypt(masterKey []byte, seqValue uint64) (uint64, error) {
	if seqValue >= (uint64(1) << blockBits) {
		return 0, fmt.Errorf("sequence overflow: %d >= 2^%d", seqValue, blockBits)
	}
	L := (seqValue >> halfBits) & mask26
	R := seqValue & mask26
	for i := 1; i <= rounds; i++ {
		rk := roundKey(masterKey, i)
		L, R = R, L^f(rk, R)
	}
	return (L << halfBits) | R, nil
}

func roundKey(masterKey []byte, round int) []byte {
	h := hmac.New(sha256.New, masterKey)
	fmt.Fprintf(h, "round-%d", round)
	return h.Sum(nil)
}

func f(rk []byte, x uint64) uint64 {
	h := hmac.New(sha256.New, rk)
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], x)
	h.Write(buf[:])
	sum := h.Sum(nil)
	// Take first 4 bytes as big-endian uint32, then mask to 26 bits.
	return uint64(binary.BigEndian.Uint32(sum[0:4])) & mask26
}
