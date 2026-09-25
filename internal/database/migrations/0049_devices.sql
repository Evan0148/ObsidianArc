-- Device identity: recognising a browser across sign-ins, independent of any
-- one session's lifetime.
--
-- A session already has an id and an IP and a user agent, but it expires and
-- is replaced constantly — signing out, a cleared cookie jar, the TTL simply
-- running out. None of that is "a new device," and none of it should look
-- like one. The device cookie is separate and long-lived precisely so it
-- outlives the sessions issued while it was presented, which is what lets
-- RecordDevice tell "the same laptop, a fresh session" from "somewhere this
-- account has never signed in before."
ALTER TABLE sessions ADD COLUMN device_id TEXT NOT NULL DEFAULT '';

-- One row per device an account has ever signed in from, keyed by the SHA-256
-- of its cookie value — the cookie itself is never stored, the same reason a
-- session token is not. last_ip and user_agent are overwritten on every visit
-- because they describe "where this device was last seen," not a history; the
-- security log is where a history belongs.
CREATE TABLE user_devices (
    user_id    TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    device_id  TEXT NOT NULL,
    first_seen BIGINT NOT NULL,
    last_seen  BIGINT NOT NULL,
    last_ip    TEXT NOT NULL DEFAULT '',
    user_agent TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (user_id, device_id)
);
