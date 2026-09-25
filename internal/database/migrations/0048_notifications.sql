-- What the interface puts in the top-right corner: a bell full of things that
-- happened without the reader watching for them.
--
-- One table for every audience rather than three, because a poll asking "what
-- is new for me" is one query either way and a UNION across three tables
-- would still have to be sorted and paged afterwards. 'user' and user_id
-- narrow to one account; 'all' reaches everyone who existed when it was
-- created (see created_at below); 'admins' reaches every administrator, or —
-- when permission is set — only the ones with that grant.
--
-- The client, never the server, turns kind + params into a sentence: the row
-- is data, not English wrapped in a table, so it reads in whichever language
-- the reader's account is set to without a migration or a second column.
CREATE TABLE notifications (
    id         TEXT PRIMARY KEY,
    audience   TEXT NOT NULL, -- 'user' | 'all' | 'admins'
    user_id    TEXT REFERENCES users (id) ON DELETE CASCADE,
    -- The admin grant this notice needs, for audience 'admins'. Empty means
    -- any administrator; a value like 'feedback' narrows to the operators who
    -- could actually act on it. Unused for the other two audiences.
    permission TEXT NOT NULL DEFAULT '',
    kind       TEXT NOT NULL,
    params     TEXT NOT NULL DEFAULT '{}',
    -- An in-app path the client can push to, e.g. /admin/feedback/<id>.
    -- Empty means the notice has nowhere to send a click.
    link       TEXT NOT NULL DEFAULT '',
    created_at BIGINT NOT NULL,
    -- What single source this notice is about, e.g. the id of the
    -- announcement that raised it, so a retraction can find and delete every
    -- notice it produced. Empty for every kind with nothing to retract — most
    -- of them, since most things a notice reports on are never undone.
    ref        TEXT NOT NULL DEFAULT ''
);

-- One partial index per audience: every read this table serves filters on
-- audience first, so a query never has to scan the rows meant for somebody
-- else's inbox.
CREATE INDEX ix_notifications_user ON notifications (user_id, created_at) WHERE audience = 'user';
CREATE INDEX ix_notifications_all ON notifications (created_at) WHERE audience = 'all';
CREATE INDEX ix_notifications_admins ON notifications (permission, created_at) WHERE audience = 'admins';
-- Retraction looks up by kind + ref rather than by audience, and only ever
-- for a row that has one, so the partial condition keeps the (very common)
-- ref-less row out of it entirely.
CREATE INDEX ix_notifications_ref ON notifications (kind, ref) WHERE ref <> '';

-- Where each account last cleared the bell. A column on `users` would have
-- meant widening that table's own SELECT list for a value only this feature
-- reads; a row here that most accounts never acquire costs nothing until the
-- bell is opened for the first time.
CREATE TABLE notification_reads (
    user_id TEXT PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    seen_at BIGINT NOT NULL DEFAULT 0
);
