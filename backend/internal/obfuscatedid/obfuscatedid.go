// Package obfuscatedid implements the keyed Feistel ID generator used by
// trakrf.generate_obfuscated_id() in the database. This Go implementation is
// the reference oracle for test vectors and the PL/pgSQL parity check.
//
// Construction: 50-bit Feistel (2 x 25-bit halves), 6 rounds, HMAC-SHA256 round
// function truncated to 25 bits, output OR'd with (1 << 50) so the value lands
// in [2^50, 2^51) — disjoint from the migrated 31-bit ID range.
package obfuscatedid

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
)

const (
	BlockBits = 50
	HalfBits  = 25
	Rounds    = 6
	Mask25    = (uint64(1) << HalfBits) - 1
	HighBit   = uint64(1) << BlockBits
)

// Encrypt maps a sequence value into a 51-bit obfuscated ID. seqValue must be
// less than 2^50 (the Feistel block size).
func Encrypt(masterKey []byte, seqValue uint64) (uint64, error) {
	if seqValue >= HighBit {
		return 0, fmt.Errorf("sequence overflow: %d >= 2^%d", seqValue, BlockBits)
	}
	L := (seqValue >> HalfBits) & Mask25
	R := seqValue & Mask25
	for i := 1; i <= Rounds; i++ {
		rk := roundKey(masterKey, i)
		L, R = R, L^f(rk, R)
	}
	return ((L << HalfBits) | R) | HighBit, nil
}

func roundKey(masterKey []byte, round int) []byte {
	h := hmac.New(sha256.New, masterKey)
	fmt.Fprintf(h, "round-%d", round)
	return h.Sum(nil)
}

func f(roundKey []byte, x uint64) uint64 {
	h := hmac.New(sha256.New, roundKey)
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], x)
	h.Write(buf[:])
	sum := h.Sum(nil)
	// Take first 4 bytes as big-endian uint32, then mask to 25 bits.
	return uint64(binary.BigEndian.Uint32(sum[0:4])) & Mask25
}
