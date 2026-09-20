-- Signing in with an account somebody already has somewhere else.
--
-- A row here is a claim by one provider that a particular subject is the
-- person holding this account. The subject is the provider's own immutable
-- identifier, never the email address: an address can be given up and handed
-- to somebody else, and a table keyed on one would hand the account over with
-- it.
--
-- The account is a foreign key rather than a copy of a name, so removing an
-- account removes the way back into it in the same statement.
CREATE TABLE oauth_identities (
    id            TEXT PRIMARY KEY,
    provider      TEXT NOT NULL,
    subject       TEXT NOT NULL,
    user_id       TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    -- What the provider said at the last sign-in, for the screen that lists
    -- the connections. Never read as identity: the subject above is.
    email         TEXT NOT NULL DEFAULT '',
    login         TEXT NOT NULL DEFAULT '',
    created_at    BIGINT NOT NULL,
    last_login_at BIGINT NOT NULL DEFAULT 0
);

-- One subject is one account. Without this, a second row for the same GitHub
-- user could point at a second account and which one you got would depend on
-- row order.
CREATE UNIQUE INDEX ux_oauth_identity_subject ON oauth_identities (provider, subject);
-- And one account holds at most one connection per provider, so "disconnect
-- GitHub" is a whole answer rather than one of several rows.
CREATE UNIQUE INDEX ux_oauth_identity_account ON oauth_identities (user_id, provider);
