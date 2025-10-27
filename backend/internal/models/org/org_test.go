package org

import (
	"testing"
)

func TestOrgStruct(t *testing.T) {
	org := Org{
		ID:     1,
		Name:   "Test Org",
		Domain: "test.example.com",
		Status: "active",
	}

	if org.ID != 1 {
		t.Errorf("expected ID 1, got %d", org.ID)
	}
	if org.Name != "Test Org" {
		t.Errorf("expected name 'Test Org', got %s", org.Name)
	}
}

func TestCreateOrgRequest(t *testing.T) {
	req := CreateOrgRequest{
		Name:         "Test Org",
		Domain:       "test.example.com",
		BillingEmail: "billing@test.com",
	}

	if req.Name == "" {
		t.Error("name should not be empty")
	}
}
