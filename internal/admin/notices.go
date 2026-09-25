package admin

import (
	"context"
	"log/slog"
	"slices"
	"time"

	"github.com/OnyxAxisOwO/ObsidianArc/internal/notify"
	"github.com/OnyxAxisOwO/ObsidianArc/internal/user"
)

// push puts one notification in the bell, after the write it reports on.
//
// Detached, with its own short timeout, rather than joined to that write: by
// the time this runs the change has committed, so there is no transaction
// left to share, and a slow notification write must not turn a change that
// happened into a request that failed.
func (h *Handlers) push(ctx context.Context, n notify.Notification) {
	if h.Notify == nil {
		return
	}
	pushCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := h.Notify.Push(pushCtx, nil, n); err != nil {
		slog.ErrorContext(ctx, "admin: notify", "error", err, "kind", n.Kind, "audience", n.Audience)
	}
}

// tellAccount is push for a notice about one account, made by somebody else.
// An administrator editing their own account already knows what they did,
// and a toast repeating it back is noise.
func (h *Handlers) tellAccount(ctx context.Context, actor user.User, accountID, kind, link string, params map[string]any) {
	if actor.ID == accountID {
		return
	}
	h.push(ctx, notify.Notification{
		Audience: notify.AudienceUser, UserID: accountID,
		Kind: kind, Params: params, Link: link,
	})
}

// accountChanges names what an administrator's edit changed, as keys the
// browser words in the reader's language. Compared before and after rather
// than read off the request, because the backoffice sends the whole form on
// every save, and "your role changed" when it did not would teach people to
// ignore the one that matters.
func accountChanges(before, after user.User) []string {
	var what []string
	if before.Nickname != after.Nickname || before.Avatar != after.Avatar || before.Bio != after.Bio ||
		before.Email != after.Email || before.QQ != after.QQ {
		what = append(what, "profile")
	}
	if before.Role != after.Role || !slices.Equal(before.AdminPermissions, after.AdminPermissions) {
		what = append(what, "role")
	}
	if before.GroupID != after.GroupID || before.GroupExpiresAt != after.GroupExpiresAt {
		what = append(what, "group")
	}
	if before.Status != after.Status {
		what = append(what, "status")
	}
	if before.APIRestricted != after.APIRestricted || before.APIRestrictedUntil != after.APIRestrictedUntil {
		what = append(what, "api")
	}
	return what
}
