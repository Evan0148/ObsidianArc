package admin

import (
	"context"
	"log/slog"
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
