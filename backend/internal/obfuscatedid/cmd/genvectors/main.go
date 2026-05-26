// genvectors writes blessed Feistel test vectors to testdata/vectors.json.
// Run via: go run ./internal/obfuscatedid/cmd/genvectors > internal/obfuscatedid/testdata/vectors.json
package main

import (
	"encoding/hex"
	"encoding/json"
	"log"
	"os"

	"github.com/trakrf/platform/backend/internal/obfuscatedid"
)

type Vector struct {
	Seq      uint64 `json:"seq"`
	Expected uint64 `json:"expected"`
}

type Bundle struct {
	MasterKeyHex string   `json:"master_key_hex"`
	Vectors      []Vector `json:"vectors"`
}

func main() {
	const keyHex = "6f626675736361746f72746573746b657920303132333435363738396162636465"
	key, err := hex.DecodeString(keyHex)
	if err != nil {
		log.Fatalf("decode hex: %v", err)
	}
	seqs := []uint64{1, 2, 100, 12345, 1 << 24, 1 << 25, 1 << 26, 1 << 49, 1 << 51, (1 << 52) - 1}
	vectors := make([]Vector, 0, len(seqs))
	for _, s := range seqs {
		id, err := obfuscatedid.Encrypt(key, s)
		if err != nil {
			log.Fatalf("Encrypt(%d): %v", s, err)
		}
		vectors = append(vectors, Vector{Seq: s, Expected: id})
	}
	bundle := Bundle{MasterKeyHex: keyHex, Vectors: vectors}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(bundle); err != nil {
		log.Fatalf("encode: %v", err)
	}
}
