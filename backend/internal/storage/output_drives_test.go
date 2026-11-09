package storage

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
)

func TestTruncateDriveError_KeepsShortText(t *testing.T) {
	assert.Equal(t, "rpc timeout", truncateDriveError("rpc timeout"))
}

func TestTruncateDriveError_CapsLongTextOnARuneBoundary(t *testing.T) {
	long := strings.Repeat("é", maxDriveErrorBytes) // 2 bytes per rune
	got := truncateDriveError(long)
	assert.LessOrEqual(t, len(got), maxDriveErrorBytes)
	assert.True(t, utf8.ValidString(got), "a cut must not split a rune")
	assert.Greater(t, len(got), maxDriveErrorBytes-4)
}
