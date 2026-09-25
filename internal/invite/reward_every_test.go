package invite

import (
	"context"
	"strconv"
	"sync"
	"testing"

	"github.com/OnyxAxisOwO/ObsidianArc/internal/settings"
	"github.com/OnyxAxisOwO/ObsidianArc/internal/user"
)

// TestRewardEveryNPaysOnlyOnMilestones is the contract's own worked example:
// with reward_every = 3, only the 3rd and 6th counted invite carry cards —
// every other counted one still resolves (rewarded_at set, reward_skipped
// empty), just with reward_cards left at 0.
func TestRewardEveryNPaysOnlyOnMilestones(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if err := f.store.settings.SetMany(ctx, map[string]string{
		settings.InvitesUserEnabled: "true", settings.InvitesRewardCards: "2",
		settings.InvitesRewardCardDays: "10", settings.InvitesUserLimit: "0",
		settings.InvitesRewardEvery: "3",
	}); err != nil {
		t.Fatalf("set settings: %v", err)
	}
	inviter := f.account(t, "milestone-inviter")
	personal, err := f.store.PersonalCode(ctx, inviter.ID)
	if err != nil {
		t.Fatalf("personal code: %v", err)
	}

	var invitees []user.User
	for i := 0; i < 6; i++ {
		invitees = append(invitees, f.registerThrough(t, personal.Code, "203.0.113."+strconv.Itoa(100+i)))
	}
	for i, invitee := range invitees {
		if err := f.store.Reward(ctx, invitee.ID, false); err != nil {
			t.Fatalf("reward invite %d: %v", i, err)
		}
		var rewardedAt int64
		var cards int
		var skipped string
		if err := f.db.QueryRow(ctx,
			`SELECT rewarded_at, reward_cards, reward_skipped FROM invite_uses WHERE user_id = ?`, invitee.ID).
			Scan(&rewardedAt, &cards, &skipped); err != nil {
			t.Fatalf("read use %d: %v", i, err)
		}
		if rewardedAt == 0 || skipped != "" {
			t.Fatalf("invite %d not counted: rewarded_at=%d reward_skipped=%q", i, rewardedAt, skipped)
		}
		wantCards := 0
		if (i+1)%3 == 0 {
			wantCards = 2
		}
		if cards != wantCards {
			t.Errorf("invite %d (counted %d): reward_cards = %d, want %d", i, i+1, cards, wantCards)
		}
	}

	held, err := f.store.cards.Available(ctx, inviter.ID)
	if err != nil {
		t.Fatalf("available cards: %v", err)
	}
	if len(held) != 4 {
		t.Fatalf("cards held = %d, want 4 (two milestones of two cards each)", len(held))
	}
}

// TestRewardNotificationCarriesCountedEveryAndRemaining checks the
// invite_joined notification's params on both a non-milestone invite and a
// milestone one.
func TestRewardNotificationCarriesCountedEveryAndRemaining(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if err := f.store.settings.SetMany(ctx, map[string]string{
		settings.InvitesUserEnabled: "true", settings.InvitesRewardCards: "1",
		settings.InvitesUserLimit: "0", settings.InvitesRewardEvery: "3",
	}); err != nil {
		t.Fatalf("set settings: %v", err)
	}
	inviter := f.account(t, "notified-inviter")
	personal, err := f.store.PersonalCode(ctx, inviter.ID)
	if err != nil {
		t.Fatalf("personal code: %v", err)
	}

	first := f.registerThrough(t, personal.Code, "203.0.113.140")
	if err := f.store.Reward(ctx, first.ID, false); err != nil {
		t.Fatalf("reward first: %v", err)
	}
	second := f.registerThrough(t, personal.Code, "203.0.113.141")
	if err := f.store.Reward(ctx, second.ID, false); err != nil {
		t.Fatalf("reward second: %v", err)
	}
	third := f.registerThrough(t, personal.Code, "203.0.113.142")
	if err := f.store.Reward(ctx, third.ID, false); err != nil {
		t.Fatalf("reward third: %v", err)
	}

	notes, err := f.notify.List(ctx, inviter, 20, 0)
	if err != nil {
		t.Fatalf("list notifications: %v", err)
	}
	// Indexed by username rather than trusting list order: three pushes this
	// close together can land in the same millisecond, and created_at DESC
	// does not break that tie in any particular direction.
	byUsername := map[string]map[string]any{}
	for _, n := range notes {
		if n.Kind == "invite_joined" {
			if username, ok := n.Params["username"].(string); ok {
				byUsername[username] = n.Params
			}
		}
	}
	if len(byUsername) != 3 {
		t.Fatalf("invite_joined notifications = %d, want 3", len(byUsername))
	}
	checkNum := func(t *testing.T, params map[string]any, key string, want int) {
		t.Helper()
		got, ok := params[key].(float64)
		if !ok || int(got) != want {
			t.Errorf("params[%q] = %v, want %d", key, params[key], want)
		}
	}
	firstParams, ok := byUsername[first.Username]
	if !ok {
		t.Fatalf("no invite_joined notification for %s", first.Username)
	}
	checkNum(t, firstParams, "counted", 1)
	checkNum(t, firstParams, "every", 3)
	checkNum(t, firstParams, "cards", 0)
	checkNum(t, firstParams, "remaining", 2)

	thirdParams, ok := byUsername[third.Username]
	if !ok {
		t.Fatalf("no invite_joined notification for %s", third.Username)
	}
	checkNum(t, thirdParams, "counted", 3)
	checkNum(t, thirdParams, "every", 3)
	checkNum(t, thirdParams, "cards", 1)
	checkNum(t, thirdParams, "remaining", 3)
}

// TestConcurrentRewardsCannotDoublePayAMilestone is the concurrency bar
// applied to the every-N cadence itself: several invitees all qualifying at
// once must not let two of them both land on the same milestone number —
// the inviter's row lock (already taken by Reward) has to serialise the
// counted-total read and the reward_cards write the same way it already
// serialises the user_limit check.
func TestConcurrentRewardsCannotDoublePayAMilestone(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if err := f.store.settings.SetMany(ctx, map[string]string{
		settings.InvitesUserEnabled: "true", settings.InvitesRewardCards: "5",
		settings.InvitesUserLimit: "0", settings.InvitesRewardEvery: "3",
	}); err != nil {
		t.Fatalf("set settings: %v", err)
	}
	inviter := f.account(t, "concurrent-milestone-inviter")
	personal, err := f.store.PersonalCode(ctx, inviter.ID)
	if err != nil {
		t.Fatalf("personal code: %v", err)
	}

	const invitees = 9 // three milestones of three at reward_every = 3
	var accounts []user.User
	for i := 0; i < invitees; i++ {
		accounts = append(accounts, f.registerThrough(t, personal.Code, "203.0.113."+strconv.Itoa(160+i)))
	}

	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make(chan error, invitees)
	for _, invitee := range accounts {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			<-start
			errs <- f.store.Reward(ctx, id, false)
		}(invitee.ID)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("reward: %v", err)
		}
	}

	// Exactly three invites (the 3rd, 6th and 9th counted, whichever
	// accounts they turned out to be) should carry cards, however the
	// goroutines interleaved — never more, never fewer.
	var milestoneRows, totalCounted int
	if err := f.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM invite_uses WHERE inviter_id = ? AND reward_cards > 0`, inviter.ID).
		Scan(&milestoneRows); err != nil {
		t.Fatalf("count milestone rows: %v", err)
	}
	if err := f.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM invite_uses WHERE inviter_id = ? AND rewarded_at <> 0 AND reward_skipped = ''`, inviter.ID).
		Scan(&totalCounted); err != nil {
		t.Fatalf("count counted rows: %v", err)
	}
	if totalCounted != invitees {
		t.Fatalf("counted invites = %d, want %d", totalCounted, invitees)
	}
	if milestoneRows != invitees/3 {
		t.Fatalf("rows carrying cards = %d, want %d", milestoneRows, invitees/3)
	}
	held, err := f.store.cards.Available(ctx, inviter.ID)
	if err != nil {
		t.Fatalf("available cards: %v", err)
	}
	if len(held) != (invitees/3)*5 {
		t.Fatalf("cards held = %d, want %d", len(held), (invitees/3)*5)
	}
}

// TestRewardMilestoneSurvivesAnInviteeBeingDeleted is the exact scenario a
// live `COUNT(*) FROM invite_uses` used to get wrong: invite_uses.user_id
// cascades away when internal/admin/people.go hard-deletes an account, and a
// milestone total recomputed from that table would then drop, letting the
// same reward_every tier pay out twice once new invitees bring the
// recomputed count back up to it. The durable users.invite_reward_count
// column must not move when an unrelated account disappears.
func TestRewardMilestoneSurvivesAnInviteeBeingDeleted(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if err := f.store.settings.SetMany(ctx, map[string]string{
		settings.InvitesUserEnabled: "true", settings.InvitesRewardCards: "2",
		settings.InvitesRewardCardDays: "10", settings.InvitesUserLimit: "0",
		settings.InvitesRewardEvery: "3",
	}); err != nil {
		t.Fatalf("set settings: %v", err)
	}
	inviter := f.account(t, "deleted-invitee-inviter")
	personal, err := f.store.PersonalCode(ctx, inviter.ID)
	if err != nil {
		t.Fatalf("personal code: %v", err)
	}

	a := f.registerThrough(t, personal.Code, "203.0.113.170")
	b := f.registerThrough(t, personal.Code, "203.0.113.171")
	c := f.registerThrough(t, personal.Code, "203.0.113.172")
	for i, invitee := range []user.User{a, b, c} {
		if err := f.store.Reward(ctx, invitee.ID, false); err != nil {
			t.Fatalf("reward invite %d: %v", i, err)
		}
	}

	held, err := f.store.cards.Available(ctx, inviter.ID)
	if err != nil {
		t.Fatalf("available cards after milestone: %v", err)
	}
	if len(held) != 2 {
		t.Fatalf("cards held after 3rd invite = %d, want 2 (the milestone paid once)", len(held))
	}

	// A hard delete of the first invitee, the way admin.instance's people
	// handler does it — this cascades A's invite_uses row away, which is
	// exactly what used to make countedInvites recompute a lower total.
	if err := f.users.Delete(ctx, nil, a.ID); err != nil {
		t.Fatalf("delete invitee a: %v", err)
	}

	var remainingUses int
	if err := f.db.QueryRow(ctx, `SELECT COUNT(*) FROM invite_uses WHERE inviter_id = ?`, inviter.ID).
		Scan(&remainingUses); err != nil {
		t.Fatalf("count remaining uses: %v", err)
	}
	if remainingUses != 2 {
		t.Fatalf("invite_uses rows for inviter after delete = %d, want 2 (cascade removed a's row)", remainingUses)
	}

	// A 4th invitee registers and is rewarded. If the total were still
	// recomputed live from invite_uses, it would read 2 (b, c) + 1 = 3 and
	// pay the milestone a second time; the durable counter must instead
	// carry the true total of 4 forward and grant nothing.
	d := f.registerThrough(t, personal.Code, "203.0.113.173")
	if err := f.store.Reward(ctx, d.ID, false); err != nil {
		t.Fatalf("reward invite d: %v", err)
	}

	var counted int
	if err := f.db.QueryRow(ctx, `SELECT invite_reward_count FROM users WHERE id = ?`, inviter.ID).
		Scan(&counted); err != nil {
		t.Fatalf("read durable counter: %v", err)
	}
	if counted != 4 {
		t.Fatalf("users.invite_reward_count = %d, want 4 (a's deletion must not roll it back)", counted)
	}

	held, err = f.store.cards.Available(ctx, inviter.ID)
	if err != nil {
		t.Fatalf("available cards after 4th invite: %v", err)
	}
	if len(held) != 2 {
		t.Fatalf("cards held after 4th invite = %d, want 2 (milestone must not pay twice)", len(held))
	}
}
