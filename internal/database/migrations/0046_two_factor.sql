-- Two-step sign-in with an authenticator app.
--
-- When an account switched it on, and zero while it is off. On the users row
-- rather than only in the table below because it is read on every request —
-- whether the operator's policy has this account enrolling before it may do
-- anything else — and the users row is already being read. Both are written
-- in one transaction, so they cannot come to disagree.
--
-- It is also the epoch a remembered browser is signed against: switching the
-- second step off and on again moves it, and every "don't ask again on this
-- browser" issued before stops working.
ALTER TABLE users ADD COLUMN two_factor_at BIGINT NOT NULL DEFAULT 0;

-- The secret the app holds, and the codes that stand in for the app.
--
-- Both secrets are sealed with a key derived from the instance secret, the
-- way a provider's API key is, so a database dump is not a set of working
-- second factors. `pending` is a secret handed out for scanning and not yet
-- confirmed by a code; it becomes `secret` only when the person proves their
-- app computes the same numbers, so a half-finished setup never locks anybody
-- out.
--
-- `last_step` is the thirty-second window of the last code accepted. A code
-- is only good for a window after it, which is what stops somebody who reads
-- a code over a shoulder from using it a second time.
--
-- `recovery` is a JSON list of keyed digests, one per unused recovery code.
CREATE TABLE two_factor (
    user_id    TEXT PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    secret     %BLOB%,
    pending    %BLOB%,
    pending_at BIGINT NOT NULL DEFAULT 0,
    last_step  BIGINT NOT NULL DEFAULT 0,
    recovery   TEXT NOT NULL DEFAULT '[]',
    created_at BIGINT NOT NULL,
    updated_at BIGINT NOT NULL
);

-- A session that has proved the password but not yet the code. Nothing but
-- the second step accepts it, and it lives minutes rather than weeks.
ALTER TABLE sessions ADD COLUMN two_factor_pending BOOLEAN NOT NULL DEFAULT FALSE;
