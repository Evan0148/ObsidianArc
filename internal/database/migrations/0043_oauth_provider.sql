-- Signing in to somebody else's site with an account here.
--
-- The other direction from 0042: there, a provider tells this server who is
-- at the browser. Here, this server is the one being asked — an application
-- somebody registers sends a person over, the person says yes, and the
-- application is handed a token it can ask "who was that" with.
--
-- Nothing here is a general API grant. A token issued through this table
-- answers the identity endpoint and nothing else; spending this instance's
-- provider credit goes through api_keys, which is a different credential with
-- a different owner and a different ceiling.

-- An application an operator has registered. The secret is stored as a digest
-- for the same reason an API key's is: the value exists once, in the response
-- that created it, and nothing here can give it back.
CREATE TABLE oauth_apps (
    id            TEXT PRIMARY KEY,
    -- What the application sends as client_id. A separate value from the row
    -- id because it is pasted into somebody else's configuration file, and a
    -- row id is this database's business.
    client_id     TEXT NOT NULL,
    -- Empty for a public application — one with nowhere to keep a secret,
    -- such as a single-page app or a phone. Those must use PKCE instead.
    secret_hash   TEXT NOT NULL DEFAULT '',
    name          TEXT NOT NULL,
    -- Shown on the consent screen, under the name. This is the one place a
    -- reader learns what they are about to let in.
    description   TEXT NOT NULL DEFAULT '',
    -- Newline-separated, and matched exactly. A prefix match or a wildcard
    -- here is how an authorisation code ends up at somebody else's server.
    redirect_uris TEXT NOT NULL,
    scopes        TEXT NOT NULL DEFAULT 'openid profile email',
    -- Skips the consent screen. For the operator's own applications, where
    -- asking permission to hand an account its own identity is a click that
    -- teaches nobody anything.
    trusted       BOOLEAN NOT NULL DEFAULT FALSE,
    disabled      BOOLEAN NOT NULL DEFAULT FALSE,
    -- Who registered it. Kept as a reference so a deleted administrator does
    -- not take a working application with them.
    created_by    TEXT REFERENCES users (id) ON DELETE SET NULL,
    created_at    BIGINT NOT NULL,
    updated_at    BIGINT NOT NULL
);

CREATE UNIQUE INDEX ux_oauth_apps_client ON oauth_apps (client_id);

-- What one account has agreed to let one application see, so that the second
-- sign-in is not a second consent screen. Removing a row is how somebody
-- takes an application's access away from their own settings.
CREATE TABLE oauth_grants (
    id           TEXT PRIMARY KEY,
    app_id       TEXT NOT NULL REFERENCES oauth_apps (id) ON DELETE CASCADE,
    user_id      TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    scopes       TEXT NOT NULL,
    created_at   BIGINT NOT NULL,
    last_used_at BIGINT NOT NULL DEFAULT 0
);

CREATE UNIQUE INDEX ux_oauth_grants ON oauth_grants (app_id, user_id);
CREATE INDEX ix_oauth_grants_user ON oauth_grants (user_id, created_at);

-- The authorisation code: one use, one minute or two, and bound to the
-- application, the callback and the PKCE challenge it was issued for. The
-- code itself is never stored — only its digest — because the row is readable
-- by anything that can read the database and the code is a way in.
CREATE TABLE oauth_codes (
    code_hash        TEXT PRIMARY KEY,
    app_id           TEXT NOT NULL REFERENCES oauth_apps (id) ON DELETE CASCADE,
    user_id          TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    redirect_uri     TEXT NOT NULL,
    scopes           TEXT NOT NULL,
    -- The application's own replay guard, returned in the identity token.
    nonce            TEXT NOT NULL DEFAULT '',
    code_challenge   TEXT NOT NULL DEFAULT '',
    challenge_method TEXT NOT NULL DEFAULT '',
    expires_at       BIGINT NOT NULL,
    -- Marked rather than deleted on exchange: a code presented twice is a
    -- stolen code, and the answer to that is to revoke what the first
    -- exchange issued, which needs the row to still be here.
    used             BOOLEAN NOT NULL DEFAULT FALSE,
    created_at       BIGINT NOT NULL
);

CREATE INDEX ix_oauth_codes_expiry ON oauth_codes (expires_at);

-- An issued token pair. Digests again, and one row for both halves: a refresh
-- rotates the pair in place, so they cannot drift apart.
CREATE TABLE oauth_tokens (
    id                 TEXT PRIMARY KEY,
    token_hash         TEXT NOT NULL,
    -- Empty when the application asked for no refresh token.
    refresh_hash       TEXT NOT NULL DEFAULT '',
    app_id             TEXT NOT NULL REFERENCES oauth_apps (id) ON DELETE CASCADE,
    user_id            TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    scopes             TEXT NOT NULL,
    expires_at         BIGINT NOT NULL,
    refresh_expires_at BIGINT NOT NULL DEFAULT 0,
    revoked            BOOLEAN NOT NULL DEFAULT FALSE,
    created_at         BIGINT NOT NULL,
    last_used_at       BIGINT NOT NULL DEFAULT 0
);

CREATE UNIQUE INDEX ux_oauth_tokens_access ON oauth_tokens (token_hash);
CREATE INDEX ix_oauth_tokens_refresh ON oauth_tokens (refresh_hash);
CREATE INDEX ix_oauth_tokens_owner ON oauth_tokens (user_id, app_id);
CREATE INDEX ix_oauth_tokens_expiry ON oauth_tokens (expires_at);

-- The key the identity tokens are signed with.
--
-- In the database rather than in the data directory, unlike the instance
-- secret and the SSH host key, because this one is published: an application
-- verifies a token against the key set this server serves, and two instances
-- against one database that had generated a key each would each reject what
-- the other signed. The private half is sealed with the instance secret, so
-- a database backup is not a key.
CREATE TABLE oauth_signing_keys (
    id          TEXT PRIMARY KEY,
    kid         TEXT NOT NULL,
    private_key %BLOB% NOT NULL,
    created_at  BIGINT NOT NULL
);
