-- Login and activity answer different questions. Seed from the evidence
-- already retained so an upgrade does not make active accounts look idle.
ALTER TABLE users ADD COLUMN last_active_at BIGINT NOT NULL DEFAULT 0;
UPDATE users SET last_active_at = last_login_at;
UPDATE users SET last_active_at = (SELECT MAX(last_seen_at) FROM sessions WHERE user_id = users.id)
WHERE EXISTS (SELECT 1 FROM sessions WHERE user_id = users.id AND last_seen_at > users.last_active_at);
UPDATE users SET last_active_at = (SELECT MAX(created_at) FROM messages WHERE conversation_id IN (SELECT id FROM conversations WHERE user_id = users.id))
WHERE EXISTS (SELECT 1 FROM messages WHERE conversation_id IN (SELECT id FROM conversations WHERE user_id = users.id) AND created_at > users.last_active_at);
