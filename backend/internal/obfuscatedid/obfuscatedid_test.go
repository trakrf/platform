package obfuscatedid

import (
	"encoding/hex"
	"testing"
)

const testMasterKeyHex = "6f626675736361746f72746573746b657920303132333435363738396162636465"

func mustDecodeHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("decode hex: %v", err)
	}
	return b
}

func TestEncrypt_HighBitAlwaysSet(t *testing.T) {
	key := mustDecodeHex(t, testMasterKeyHex)
	for _, seq := range []uint64{1, 2, 100, 1 << 24, (1 << 50) - 1} {
		id, err := Encrypt(key, seq)
		if err != nil {
			t.Fatalf("Encrypt(%d): unexpected error %v", seq, err)
		}
		if id < (1 << 50) {
			t.Errorf("Encrypt(%d) = %d, expected >= 2^50", seq, id)
		}
		if id >= (1 << 51) {
			t.Errorf("Encrypt(%d) = %d, expected < 2^51", seq, id)
		}
	}
}

func TestEncrypt_OverflowError(t *testing.T) {
	key := mustDecodeHex(t, testMasterKeyHex)
	_, err := Encrypt(key, 1<<50)
	if err == nil {
		t.Error("Encrypt(2^50) should return overflow error")
	}
}

func TestEncrypt_Bijection(t *testing.T) {
	key := mustDecodeHex(t, testMasterKeyHex)
	const N = 10_000
	seen := make(map[uint64]uint64, N)
	for seq := uint64(1); seq <= N; seq++ {
		id, err := Encrypt(key, seq)
		if err != nil {
			t.Fatalf("Encrypt(%d): %v", seq, err)
		}
		if prev, ok := seen[id]; ok {
			t.Fatalf("collision: Encrypt(%d) == Encrypt(%d) == %d", seq, prev, id)
		}
		seen[id] = seq
	}
}
