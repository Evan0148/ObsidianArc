-- A report is a conversation, not a form submission.
--
-- The first version could only answer with a status, which told somebody
-- their report had been dealt with and nothing about how. Both sides write
-- here, and both write Markdown: a defect report is code and steps, and an
-- answer is often a link and a version number.
--
-- Replies cascade with their author, exactly as the report itself does.
-- Deleting an administrator therefore takes their answers with them, which
-- is the same rule applied consistently rather than a separate one invented
-- for staff — and the reader's own words, which are the record that matters,
-- belong to an account that is still there.
CREATE TABLE feedback_replies (
    id          TEXT PRIMARY KEY,
    feedback_id TEXT NOT NULL REFERENCES feedback (id) ON DELETE CASCADE,
    user_id     TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    -- Which side spoke, recorded at the moment it was written rather than
    -- derived from the author's role now: somebody who answers reports today
    -- and loses the grant tomorrow still said it as an operator.
    from_staff  BOOLEAN NOT NULL DEFAULT FALSE,
    body        TEXT NOT NULL,
    created_at  BIGINT NOT NULL
);

-- One thread, oldest first, is the only way this table is ever read.
CREATE INDEX ix_feedback_replies_thread ON feedback_replies (feedback_id, created_at);

-- Which side has not yet read the other's last word. Two flags rather than a
-- pair of timestamps per reader: a thread has exactly two sides, and "has the
-- other one seen this" is the whole question either of them asks.
ALTER TABLE feedback ADD COLUMN author_unread BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE feedback ADD COLUMN operator_unread BOOLEAN NOT NULL DEFAULT FALSE;
