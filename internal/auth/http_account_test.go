package auth

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/OnyxAxisOwO/ObsidianArc/internal/group"
	"github.com/OnyxAxisOwO/ObsidianArc/internal/user"
)

func TestAccountPayloadCarriesGroupCopy(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	membership, err := f.groups.Default(ctx, nil)
	if err != nil {
		t.Fatalf("load default group: %v", err)
	}
	description := "**Private beta** members get early access."
	if _, err := f.groups.Update(ctx, nil, membership.ID, group.Update{Description: &description}); err != nil {
		t.Fatalf("describe group: %v", err)
	}

	handler := &Handlers{groups: f.groups}
	request := httptest.NewRequest("GET", "/api/auth/me", nil)
	payload := handler.account(request, user.User{GroupID: membership.ID})

	if payload.GroupName != membership.Name {
		t.Errorf("group name = %q, want %q", payload.GroupName, membership.Name)
	}
	if payload.GroupDescription != description {
		t.Errorf("group description = %q, want %q", payload.GroupDescription, description)
	}
}
