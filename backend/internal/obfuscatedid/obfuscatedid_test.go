package obfuscatedid

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
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

func TestEncrypt_OutputInBlockRange(t *testing.T) {
	key := mustDecodeHex(t, testMasterKeyHex)
	for _, seq := range []uint64{1, 2, 100, 1 << 24, (1 << 52) - 1} {
		id, err := Encrypt(key, seq)
		if err != nil {
			t.Fatalf("Encrypt(%d): unexpected error %v", seq, err)
		}
		if id >= (1 << 52) {
			t.Errorf("Encrypt(%d) = %d, expected < 2^52", seq, id)
		}
	}
}

func TestEncrypt_OverflowError(t *testing.T) {
	key := mustDecodeHex(t, testMasterKeyHex)
	_, err := Encrypt(key, 1<<52)
	if err == nil {
		t.Error("Encrypt(2^52) should return overflow error")
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

type testVector struct {
	Seq      uint64 `json:"seq"`
	Expected uint64 `json:"expected"`
}

type testBundle struct {
	MasterKeyHex string       `json:"master_key_hex"`
	Vectors      []testVector `json:"vectors"`
}

func loadTestBundle(t *testing.T) testBundle {
	t.Helper()
	path := filepath.Join("testdata", "vectors.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read vectors: %v", err)
	}
	var b testBundle
	if err := json.Unmarshal(data, &b); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return b
}

func TestEncrypt_AgainstBlessedVectors(t *testing.T) {
	b := loadTestBundle(t)
	key := mustDecodeHex(t, b.MasterKeyHex)
	for _, v := range b.Vectors {
		got, err := Encrypt(key, v.Seq)
		if err != nil {
			t.Fatalf("Encrypt(%d): %v", v.Seq, err)
		}
		if got != v.Expected {
			t.Errorf("Encrypt(%d) = %d, want %d", v.Seq, got, v.Expected)
		}
	}
}
