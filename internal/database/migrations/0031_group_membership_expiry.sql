-- Zero preserves permanent memberships; expiry returns to the default group
-- configured at that time rather than remembering a possibly deleted group.
ALTER TABLE users ADD COLUMN group_expires_at BIGINT NOT NULL DEFAULT 0;
CREATE INDEX users_group_expiry ON users (group_expires_at);
