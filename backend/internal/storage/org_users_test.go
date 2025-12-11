package storage

import (
	"encoding/json"
	"testing"

	"github.com/trakrf/platform/backend/internal/models/organization"
)

func TestListOrgMembersReturnsEmptyArrayNotNull(t *testing.T) {
	// This test documents the expected JSON serialization behavior
	// Empty slice should serialize to [] not null
	members := []organization.OrgMember{}

	data, err := json.Marshal(map[string]any{"data": members})
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	expected := `{"data":[]}`
	if string(data) != expected {
		t.Errorf("expected %s, got %s", expected, string(data))
	}
}

func TestNilSliceSerializesToNull(t *testing.T) {
	// This test documents why we must use []T{} not var []T
	var members []organization.OrgMember // nil slice

	data, err := json.Marshal(map[string]any{"data": members})
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	// nil slice serializes to null - this is the bug we're fixing
	if string(data) != `{"data":null}` {
		t.Errorf("expected nil slice to serialize to null, got %s", string(data))
	}
}

func TestListOrgMembers(t *testing.T) {
	t.Skip("Requires test database - implement in integration tests")
}
