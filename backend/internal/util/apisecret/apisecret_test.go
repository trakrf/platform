package apisecret

import (
	"strings"
	"testing"
)

func TestGenerateProducesPrefixedUniqueSecrets(t *testing.T) {
	a, err := Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	b, err := Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !strings.HasPrefix(a, "trakrf_") {
		t.Errorf("missing trakrf_ prefix: %q", a)
	}
	if a == b {
		t.Error("two Generate() calls returned identical secrets")
	}
	// trakrf_ (7) + 64 hex chars
	if len(a) != 7+64 {
		t.Errorf("unexpected length %d: %q", len(a), a)
	}
}

func TestHashIsStableAndVerifies(t *testing.T) {
	secret, _ := Generate()
	h := Hash(secret)
	if len(h) != 64 {
		t.Errorf("hash not 64 hex chars: %q", h)
	}
	if Hash(secret) != h {
		t.Error("Hash not deterministic")
	}
	if !Verify(secret, h) {
		t.Error("Verify rejected the correct secret")
	}
	if Verify(secret+"x", h) {
		t.Error("Verify accepted a wrong secret")
	}
	if Verify("", h) {
		t.Error("Verify accepted empty secret")
	}
}
