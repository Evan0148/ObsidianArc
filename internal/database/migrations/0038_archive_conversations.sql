-- Conversations can be archived to tidy the active history without discarding them.
--
-- An archived conversation remains readable on demand, but is excluded from the
-- default list until it is restored.
ALTER TABLE conversations ADD COLUMN archived BOOLEAN NOT NULL DEFAULT FALSE;

CREATE INDEX ix_conversations_user_archived ON conversations (user_id, archived, updated_at);
