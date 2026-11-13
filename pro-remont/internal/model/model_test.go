package model

import "testing"

func TestCanTransition(t *testing.T) {
	tests := []struct {
		from, to string
		want     bool
	}{
		{StatusNew, StatusAccepted, true},
		{StatusAccepted, StatusAssigned, true},
		{StatusAssigned, StatusInProgress, true},
		{StatusInProgress, StatusReady, true},
		{StatusReady, StatusCompleted, true},
		{StatusCompleted, StatusPaid, true},
		{StatusNew, StatusReady, false},
		{StatusPaid, StatusCompleted, false},
		{StatusCancelled, StatusNew, false},
		{StatusNew, StatusNew, false},
	}
	for _, tt := range tests {
		if got := CanTransition(tt.from, tt.to); got != tt.want {
			t.Errorf("CanTransition(%q, %q) = %v, want %v", tt.from, tt.to, got, tt.want)
		}
	}
}

func TestCanRoleSetStatus(t *testing.T) {
	tests := []struct {
		role, from, to string
		want           bool
	}{
		{RoleMaster, StatusAssigned, StatusInProgress, true},
		{RoleMaster, StatusInProgress, StatusReady, true},
		{RoleMaster, StatusReady, StatusCompleted, false},
		{RoleManager, StatusNew, StatusAccepted, true},
		{RoleManager, StatusAccepted, StatusAssigned, true},
		{RoleManager, StatusReady, StatusPaid, true},
		{RoleClient, StatusNew, StatusCancelled, true},
		{RoleClient, StatusAccepted, StatusCancelled, true},
		{RoleClient, StatusInProgress, StatusCancelled, false},
		{RoleAdmin, StatusNew, StatusCancelled, true},
	}
	for _, tt := range tests {
		if got := CanRoleSetStatus(tt.role, tt.from, tt.to); got != tt.want {
			t.Errorf("CanRoleSetStatus(%q, %q, %q) = %v, want %v", tt.role, tt.from, tt.to, got, tt.want)
		}
	}
}

func TestPermissions(t *testing.T) {
	admin := &User{Role: RoleAdmin, IsActive: true}
	if !HasPermission(admin, PermManageUsers) {
		t.Fatal("admin must have every permission")
	}
	manager := &User{Role: RoleManager, IsActive: true, Permissions: []string{PermManageRequests}}
	if !HasPermission(manager, PermManageRequests) {
		t.Fatal("manager must have granted permission")
	}
	if HasPermission(manager, PermManageUsers) {
		t.Fatal("manager must not have ungranted permission")
	}
	if HasPermission(&User{Role: RoleManager, IsActive: false}, PermManageRequests) {
		t.Fatal("blocked user must not have permissions")
	}
}
