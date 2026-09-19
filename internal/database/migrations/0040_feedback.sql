-- What the people using the instance have to say about it.
--
-- Rows rather than mail, because the operator's answer to "what is broken"
-- should survive a mailbox, and because a report is worth triaging in the
-- same place the accounts and the logs already are.
--
-- The author is a foreign key rather than a copied name: a report read six
-- months later should show who can still be replied to, and a deleted
-- account's reports go with it.
CREATE TABLE feedback (
    id         TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    kind       TEXT NOT NULL,
    priority   TEXT NOT NULL,
    status     TEXT NOT NULL DEFAULT 'open',
    title      TEXT NOT NULL,
    body       TEXT NOT NULL,
    created_at BIGINT NOT NULL,
    updated_at BIGINT NOT NULL
);

-- The list is read newest first, and filtered by exactly these three columns.
CREATE INDEX ix_feedback_created ON feedback (created_at);
CREATE INDEX ix_feedback_status ON feedback (status, created_at);
CREATE INDEX ix_feedback_user ON feedback (user_id, created_at);
