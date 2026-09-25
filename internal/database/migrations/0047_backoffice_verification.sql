-- When this session last proved the second factor for the backoffice, or last
-- used the backoffice after proving it. Zero means it has not, or has left.
--
-- On the session rather than the account because the question is about one
-- browser: an administrator who typed a code at home has not unlocked the
-- backoffice in a tab somebody else is sitting at. It only matters when the
-- operator has asked for a code on every entry to the backoffice.
ALTER TABLE sessions ADD COLUMN backoffice_at BIGINT NOT NULL DEFAULT 0;
