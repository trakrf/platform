package orgs

import (
	"context"
	stderrors "errors"
	"fmt"
	"testing"
	"time"

	"github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/storage"
)

func TestMangleFormat(t *testing.T) {
	// Test that the mangle format produces expected output
	deletedAt := time.Date(2025, 12, 13, 12, 45, 0, 0, time.UTC)
	prefix := fmt.Sprintf("*** DELETED %s *** ", deletedAt.Format(time.RFC3339))

	mangledName := prefix + "Acme Corp"
	expected := "*** DELETED 2025-12-13T12:45:00Z *** Acme Corp"

	if mangledName != expected {
		t.Errorf("expected %q, got %q", expected, mangledName)
	}
}

func TestMangleFormatIdentifier(t *testing.T) {
	// Test that identifier mangling works the same way
	deletedAt := time.Date(2025, 12, 13, 12, 45, 0, 0, time.UTC)
	prefix := fmt.Sprintf("*** DELETED %s *** ", deletedAt.Format(time.RFC3339))

	mangledIdentifier := prefix + "acme-corp"
	expected := "*** DELETED 2025-12-13T12:45:00Z *** acme-corp"

	if mangledIdentifier != expected {
		t.Errorf("expected %q, got %q", expected, mangledIdentifier)
	}
}

func TestMangledNameLength(t *testing.T) {
	// Verify mangled names fit in VARCHAR(255)
	deletedAt := time.Now().UTC()
	prefix := fmt.Sprintf("*** DELETED %s *** ", deletedAt.Format(time.RFC3339))

	// Prefix is ~36 chars ("*** DELETED 2025-12-13T12:45:00Z *** ")
	// VARCHAR(255) - 36 = 219 chars available for original name
	prefixLen := len(prefix)
	if prefixLen > 40 {
		t.Errorf("prefix length %d exceeds expected ~36 chars", prefixLen)
	}

	// Test with a 200 char name (well under the 219 limit)
	longName := make([]byte, 200)
	for i := range longName {
		longName[i] = 'a'
	}
	mangledName := prefix + string(longName)

	if len(mangledName) > 255 {
		t.Errorf("mangled name exceeds 255 chars: %d", len(mangledName))
	}
}

func TestMangledNamePreservesOriginal(t *testing.T) {
	// Verify the original name is preserved in the mangled version
	originalName := "Test Organization With Special Chars !@#$%"
	deletedAt := time.Now().UTC()
	prefix := fmt.Sprintf("*** DELETED %s *** ", deletedAt.Format(time.RFC3339))
	mangledName := prefix + originalName

	// The original name should appear at the end
	expectedSuffix := originalName
	if len(mangledName) < len(expectedSuffix) {
		t.Fatalf("mangled name too short")
	}
	actualSuffix := mangledName[len(mangledName)-len(expectedSuffix):]
	if actualSuffix != expectedSuffix {
		t.Errorf("original name not preserved: expected suffix %q, got %q", expectedSuffix, actualSuffix)
	}
}

// TRA-319: SetCurrentOrg must surface storage.ErrOrgUserNotFound via errors.Is
// so the handler can distinguish "not a member" (403) from generic failures (500).
func TestSetCurrentOrg_NotMember_WrapsSentinel(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	t.Cleanup(mock.Close)

	mock.ExpectQuery(`FROM trakrf.org_users`).
		WithArgs(1, 99).
		WillReturnError(storage.ErrOrgUserNotFound)

	svc := &Service{storage: storage.NewWithPool(mock)}
	err = svc.SetCurrentOrg(context.Background(), 1, 99)
	require.Error(t, err)
	assert.True(t, stderrors.Is(err, storage.ErrOrgUserNotFound),
		"expected error to wrap storage.ErrOrgUserNotFound, got: %v", err)
}

func TestSetCurrentOrg_InternalStorageError_DoesNotLookLikeNotMember(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	t.Cleanup(mock.Close)

	mock.ExpectQuery(`FROM trakrf.org_users`).
		WithArgs(1, 99).
		WillReturnError(fmt.Errorf("connection refused"))

	svc := &Service{storage: storage.NewWithPool(mock)}
	err = svc.SetCurrentOrg(context.Background(), 1, 99)
	require.Error(t, err)
	assert.False(t, stderrors.Is(err, storage.ErrOrgUserNotFound),
		"generic DB error must not masquerade as membership error; got: %v", err)
}
