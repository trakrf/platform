package models

import (
	"testing"
)

func TestAllRoles(t *testing.T) {
	roles := AllRoles()
	if len(roles) != 4 {
		t.Errorf("expected 4 roles, got %d", len(roles))
	}

	// Verify order is by increasing privilege
	expected := []OrgRole{RoleViewer, RoleOperator, RoleManager, RoleAdmin}
	for i, role := range roles {
		if role != expected[i] {
			t.Errorf("role at index %d: expected %s, got %s", i, expected[i], role)
		}
	}
}

func TestOrgRole_IsValid(t *testing.T) {
	tests := []struct {
		role  OrgRole
		valid bool
	}{
		{RoleViewer, true},
		{RoleOperator, true},
		{RoleManager, true},
		{RoleAdmin, true},
		{OrgRole("invalid"), false},
		{OrgRole(""), false},
		{OrgRole("owner"), false},
		{OrgRole("ADMIN"), false}, // Case sensitive
	}

	for _, tt := range tests {
		t.Run(string(tt.role), func(t *testing.T) {
			if got := tt.role.IsValid(); got != tt.valid {
				t.Errorf("OrgRole(%q).IsValid() = %v, want %v", tt.role, got, tt.valid)
			}
		})
	}
}

func TestOrgRole_String(t *testing.T) {
	tests := []struct {
		role OrgRole
		want string
	}{
		{RoleViewer, "viewer"},
		{RoleOperator, "operator"},
		{RoleManager, "manager"},
		{RoleAdmin, "admin"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.role.String(); got != tt.want {
				t.Errorf("OrgRole.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOrgRole_CanView(t *testing.T) {
	// All valid roles can view
	for _, role := range AllRoles() {
		if !role.CanView() {
			t.Errorf("%s should be able to view", role)
		}
	}

	// Invalid role cannot view
	invalid := OrgRole("invalid")
	if invalid.CanView() {
		t.Error("invalid role should not be able to view")
	}
}

func TestOrgRole_CanScan(t *testing.T) {
	tests := []struct {
		role    OrgRole
		canScan bool
	}{
		{RoleViewer, false},
		{RoleOperator, true},
		{RoleManager, true},
		{RoleAdmin, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.role), func(t *testing.T) {
			if got := tt.role.CanScan(); got != tt.canScan {
				t.Errorf("%s.CanScan() = %v, want %v", tt.role, got, tt.canScan)
			}
		})
	}
}

func TestOrgRole_CanManageAssets(t *testing.T) {
	tests := []struct {
		role            OrgRole
		canManageAssets bool
	}{
		{RoleViewer, false},
		{RoleOperator, false},
		{RoleManager, true},
		{RoleAdmin, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.role), func(t *testing.T) {
			if got := tt.role.CanManageAssets(); got != tt.canManageAssets {
				t.Errorf("%s.CanManageAssets() = %v, want %v", tt.role, got, tt.canManageAssets)
			}
		})
	}
}

func TestOrgRole_CanExportReports(t *testing.T) {
	tests := []struct {
		role             OrgRole
		canExportReports bool
	}{
		{RoleViewer, false},
		{RoleOperator, false},
		{RoleManager, true},
		{RoleAdmin, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.role), func(t *testing.T) {
			if got := tt.role.CanExportReports(); got != tt.canExportReports {
				t.Errorf("%s.CanExportReports() = %v, want %v", tt.role, got, tt.canExportReports)
			}
		})
	}
}

func TestOrgRole_CanManageUsers(t *testing.T) {
	tests := []struct {
		role           OrgRole
		canManageUsers bool
	}{
		{RoleViewer, false},
		{RoleOperator, false},
		{RoleManager, false},
		{RoleAdmin, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.role), func(t *testing.T) {
			if got := tt.role.CanManageUsers(); got != tt.canManageUsers {
				t.Errorf("%s.CanManageUsers() = %v, want %v", tt.role, got, tt.canManageUsers)
			}
		})
	}
}

func TestOrgRole_CanManageOrg(t *testing.T) {
	tests := []struct {
		role         OrgRole
		canManageOrg bool
	}{
		{RoleViewer, false},
		{RoleOperator, false},
		{RoleManager, false},
		{RoleAdmin, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.role), func(t *testing.T) {
			if got := tt.role.CanManageOrg(); got != tt.canManageOrg {
				t.Errorf("%s.CanManageOrg() = %v, want %v", tt.role, got, tt.canManageOrg)
			}
		})
	}
}

func TestOrgRole_HasAtLeast(t *testing.T) {
	tests := []struct {
		role    OrgRole
		minRole OrgRole
		want    bool
	}{
		// Viewer tests
		{RoleViewer, RoleViewer, true},
		{RoleViewer, RoleOperator, false},
		{RoleViewer, RoleManager, false},
		{RoleViewer, RoleAdmin, false},

		// Operator tests
		{RoleOperator, RoleViewer, true},
		{RoleOperator, RoleOperator, true},
		{RoleOperator, RoleManager, false},
		{RoleOperator, RoleAdmin, false},

		// Manager tests
		{RoleManager, RoleViewer, true},
		{RoleManager, RoleOperator, true},
		{RoleManager, RoleManager, true},
		{RoleManager, RoleAdmin, false},

		// Admin tests
		{RoleAdmin, RoleViewer, true},
		{RoleAdmin, RoleOperator, true},
		{RoleAdmin, RoleManager, true},
		{RoleAdmin, RoleAdmin, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.role)+"_has_"+string(tt.minRole), func(t *testing.T) {
			if got := tt.role.HasAtLeast(tt.minRole); got != tt.want {
				t.Errorf("%s.HasAtLeast(%s) = %v, want %v", tt.role, tt.minRole, got, tt.want)
			}
		})
	}
}

func TestOrgRole_Scan(t *testing.T) {
	tests := []struct {
		name    string
		input   interface{}
		want    OrgRole
		wantErr bool
	}{
		{"string viewer", "viewer", RoleViewer, false},
		{"string operator", "operator", RoleOperator, false},
		{"string manager", "manager", RoleManager, false},
		{"string admin", "admin", RoleAdmin, false},
		{"bytes viewer", []byte("viewer"), RoleViewer, false},
		{"bytes admin", []byte("admin"), RoleAdmin, false},
		{"nil defaults to viewer", nil, RoleViewer, false},
		{"invalid role", "invalid", "", true},
		{"wrong type", 123, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var r OrgRole
			err := r.Scan(tt.input)

			if (err != nil) != tt.wantErr {
				t.Errorf("Scan() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && r != tt.want {
				t.Errorf("Scan() got = %v, want %v", r, tt.want)
			}
		})
	}
}

func TestOrgRole_Value(t *testing.T) {
	tests := []struct {
		role    OrgRole
		want    string
		wantErr bool
	}{
		{RoleViewer, "viewer", false},
		{RoleOperator, "operator", false},
		{RoleManager, "manager", false},
		{RoleAdmin, "admin", false},
		{OrgRole("invalid"), "", true},
	}

	for _, tt := range tests {
		t.Run(string(tt.role), func(t *testing.T) {
			got, err := tt.role.Value()

			if (err != nil) != tt.wantErr {
				t.Errorf("Value() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if gotStr, ok := got.(string); !ok || gotStr != tt.want {
					t.Errorf("Value() = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

// TestPermissionHierarchy validates that higher roles have all permissions of lower roles
func TestPermissionHierarchy(t *testing.T) {
	// Define permissions in order of increasing privilege
	permissions := []struct {
		name  string
		check func(OrgRole) bool
	}{
		{"CanView", func(r OrgRole) bool { return r.CanView() }},
		{"CanScan", func(r OrgRole) bool { return r.CanScan() }},
		{"CanManageAssets", func(r OrgRole) bool { return r.CanManageAssets() }},
		{"CanExportReports", func(r OrgRole) bool { return r.CanExportReports() }},
		{"CanManageUsers", func(r OrgRole) bool { return r.CanManageUsers() }},
		{"CanManageOrg", func(r OrgRole) bool { return r.CanManageOrg() }},
	}

	roles := AllRoles()

	// For each permission, if a role has it, all higher roles should too
	for _, perm := range permissions {
		for i, role := range roles {
			if perm.check(role) {
				// All roles at index i and above should have this permission
				for j := i; j < len(roles); j++ {
					if !perm.check(roles[j]) {
						t.Errorf("role %s has %s but higher role %s does not",
							role, perm.name, roles[j])
					}
				}
			}
		}
	}
}
