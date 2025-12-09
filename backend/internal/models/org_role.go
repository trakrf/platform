package models

import (
	"database/sql/driver"
	"fmt"
)

// OrgRole represents a user's role within an organization
type OrgRole string

const (
	RoleViewer   OrgRole = "viewer"
	RoleOperator OrgRole = "operator"
	RoleManager  OrgRole = "manager"
	RoleAdmin    OrgRole = "admin"
)

// AllRoles returns all valid org roles in order of increasing privilege
func AllRoles() []OrgRole {
	return []OrgRole{RoleViewer, RoleOperator, RoleManager, RoleAdmin}
}

// IsValid checks if the role is a valid OrgRole value
func (r OrgRole) IsValid() bool {
	switch r {
	case RoleViewer, RoleOperator, RoleManager, RoleAdmin:
		return true
	}
	return false
}

// String returns the string representation of the role
func (r OrgRole) String() string {
	return string(r)
}

// Permission checks

// CanView returns true if the role can view assets and locations
func (r OrgRole) CanView() bool {
	return r.IsValid() // All valid roles can view
}

// CanScan returns true if the role can run and save scans
func (r OrgRole) CanScan() bool {
	return r == RoleOperator || r == RoleManager || r == RoleAdmin
}

// CanManageAssets returns true if the role can create/edit assets and locations
func (r OrgRole) CanManageAssets() bool {
	return r == RoleManager || r == RoleAdmin
}

// CanExportReports returns true if the role can export reports
func (r OrgRole) CanExportReports() bool {
	return r == RoleManager || r == RoleAdmin
}

// CanManageUsers returns true if the role can invite/remove users and change roles
func (r OrgRole) CanManageUsers() bool {
	return r == RoleAdmin
}

// CanManageOrg returns true if the role can edit org settings and delete org
func (r OrgRole) CanManageOrg() bool {
	return r == RoleAdmin
}

// HasAtLeast returns true if this role has at least the permissions of minRole
func (r OrgRole) HasAtLeast(minRole OrgRole) bool {
	roleOrder := map[OrgRole]int{
		RoleViewer:   1,
		RoleOperator: 2,
		RoleManager:  3,
		RoleAdmin:    4,
	}
	return roleOrder[r] >= roleOrder[minRole]
}

// Scan implements sql.Scanner for database reads
func (r *OrgRole) Scan(value interface{}) error {
	if value == nil {
		*r = RoleViewer
		return nil
	}
	switch v := value.(type) {
	case string:
		*r = OrgRole(v)
	case []byte:
		*r = OrgRole(string(v))
	default:
		return fmt.Errorf("cannot scan %T into OrgRole", value)
	}
	if !r.IsValid() {
		return fmt.Errorf("invalid org role: %s", *r)
	}
	return nil
}

// Value implements driver.Valuer for database writes
func (r OrgRole) Value() (driver.Value, error) {
	if !r.IsValid() {
		return nil, fmt.Errorf("invalid org role: %s", r)
	}
	return string(r), nil
}
