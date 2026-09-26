package auth

import (
	"context"
	"errors"
	"testing"
)

func TestUpdateWorkspaceRecordsAuditAndFailsClosed(t *testing.T) {
	repo := newMemoryRepository()
	store := NewStore(repo)
	user, workspace, _, err := store.Register(context.Background(), "owner@example.com", "password1", "Owner", "Workspace", SessionMeta{})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	if err := store.UpdateWorkspace(context.Background(), workspace.ID, user.Email, "Renamed"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	audit, ok := repo.lastAudit()
	if !ok || audit.Action != "org.rename" || audit.Before != `{"name":"Workspace"}` || audit.After != `{"name":"Renamed"}` {
		t.Fatalf("audit = %#v, want org.rename with before/after snapshots", audit)
	}

	repo.auditErr = errors.New("audit unavailable")
	if err := store.UpdateWorkspace(context.Background(), workspace.ID, user.Email, "Rejected"); !errors.Is(err, repo.auditErr) {
		t.Fatalf("failed audit error = %v, want %v", err, repo.auditErr)
	}
	current, err := repo.GetOrgByID(context.Background(), workspace.ID)
	if err != nil {
		t.Fatalf("get after rollback: %v", err)
	}
	if current.Name != "Renamed" {
		t.Fatalf("name after failed audit = %q, want Renamed", current.Name)
	}
}
