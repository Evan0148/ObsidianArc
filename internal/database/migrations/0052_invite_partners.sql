-- Partner codes and existing-account claims.
--
-- 0051 gave every code an owner_id that is either an admin batch ('') or one
-- account's own personal code ('' vs the account). A partner's code sits in
-- neither box — nobody owns it the way a personal code is owned, but it is
-- named and tracked the way a plain batch never needs to be — so kind and
-- name tell the three shapes apart explicitly instead of inferring "partner"
-- from "owner_id is empty but somebody typed a custom code," which an
-- ordinary batch of one custom code also satisfies.
ALTER TABLE invite_codes ADD COLUMN kind TEXT NOT NULL DEFAULT 'batch';
ALTER TABLE invite_codes ADD COLUMN name TEXT NOT NULL DEFAULT '';
-- Whether an account that already exists may redeem this code through
-- POST /api/profile/invites/claim, on top of whoever registers through it.
-- Off for an ordinary batch, on by default for a partner (see
-- invite.CreateInput) — a partner's whole point is being handed to people
-- who already have an account here, not only to strangers signing up.
ALTER TABLE invite_codes ADD COLUMN allow_existing BOOLEAN NOT NULL DEFAULT FALSE;

-- Every code 0051 could produce was either an admin batch (owner_id = '') or
-- a personal code (owner_id = the account); nothing before this migration
-- ever set owner_id to anything else, so this is exactly the set
-- Store.ownedCode and PersonalCode have always meant by "this account's own
-- code."
UPDATE invite_codes SET kind = 'personal' WHERE owner_id <> '';

-- One row per account that redeemed a batch or partner code it did not
-- register through — Claim's own ledger, parallel to invite_uses but never
-- confused with it: a registration seats a brand new account and may earn
-- its inviter a reward, a claim adds time to an account that already exists
-- and rewards nobody. The primary key is what makes "one claim per account
-- per code" an invariant the database enforces rather than a race Claim has
-- to avoid on its own.
CREATE TABLE invite_claims (
    code_id    TEXT NOT NULL,
    user_id    TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    -- What this particular claim actually drew. Kept per-row for the same
    -- reason invite_uses keeps its own group_days: group_days_max makes it a
    -- random choice at claim time, and the exact number granted is worth
    -- keeping rather than recomputing from a range that may since have
    -- changed.
    group_days INTEGER NOT NULL DEFAULT 0,
    created_at BIGINT NOT NULL,
    PRIMARY KEY (code_id, user_id)
);

-- An account's own claim history, and — alongside invite_uses — the "used"
-- and "claims" figures the admin invites table and GET /api/profile/invites
-- read back.
CREATE INDEX ix_invite_claims_code ON invite_claims (code_id);
CREATE INDEX ix_invite_claims_user ON invite_claims (user_id);

-- Claim's own rate limiter: ten failed attempts in a rolling hour block an
-- account from trying an eleventh — the same shape auth.Limiter enforces for
-- signing in, kept here instead of reusing that one because it is in-memory
-- and keyed by IP and identifier for a stranger nobody has authenticated
-- yet. The account calling Claim is already signed in and already has a row,
-- so its cap can be the row-locked check-then-write AGENTS.md asks for
-- instead of a second in-memory table one process would forget on every
-- restart.
CREATE TABLE invite_claim_throttle (
    user_id      TEXT PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    window_start BIGINT NOT NULL DEFAULT 0,
    failures     INTEGER NOT NULL DEFAULT 0
);

-- The inviter's durable running total of qualifying (counted) invites,
-- persisted on the account's own row instead of recomputed with
-- `SELECT COUNT(*) FROM invite_uses WHERE ...` on every reward. invite_uses
-- rows cascade away when the invitee's account is later hard-deleted
-- (invite_uses.user_id REFERENCES users(id) ON DELETE CASCADE, 0051), which
-- would shift a live COUNT(*) down and let an every-N milestone that was
-- already paid be paid a second time once enough new invitees replace the
-- deleted count. A column written once, under the inviter's own row lock, at
-- the moment an invite is counted, only ever grows — deleting some other
-- account has nothing to decrement.
ALTER TABLE users ADD COLUMN invite_reward_count INTEGER NOT NULL DEFAULT 0;

-- Backfill from whatever invite_uses already recorded before this column
-- existed, so a reward already counted under the old live-COUNT(*) scheme
-- is not lost or replayed the first time the new column is read.
UPDATE users SET invite_reward_count = (
    SELECT COUNT(*) FROM invite_uses
    WHERE invite_uses.inviter_id = users.id
      AND invite_uses.rewarded_at <> 0 AND invite_uses.reward_skipped = ''
);
