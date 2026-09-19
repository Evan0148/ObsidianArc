-- Projects, and the two surfaces a conversation can belong to.
--
-- A project is a name and a standing prompt that the conversations started
-- inside it inherit. It exists because the work surface produces a different
-- kind of transcript from the chat one — a record of what was done to the
-- instance rather than a conversation — and those two piling into one rail,
-- under one history, is how an operational record gets lost among the chat.
--
-- user_id is NOT NULL and every read is scoped by it. That is a security
-- boundary, not tidiness: a project's instructions become part of the system
-- prompt of a session that, on the work surface, is holding real tools.
-- Sharing a project would be letting its author write instructions for an
-- agent running as somebody else, with somebody else's permissions. There is
-- no sharing in this version, and this column is what stops one being added
-- without the question being asked again.
CREATE TABLE projects (
    id           TEXT PRIMARY KEY,
    user_id      TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    -- Appended to the prompt chain, never a replacement for it. The operator's
    -- instance prompt is where the rules a user must not switch off live, and
    -- this text is written by that user.
    instructions TEXT NOT NULL DEFAULT '',
    created_at   BIGINT NOT NULL,
    updated_at   BIGINT NOT NULL
);

-- The rail's own query: this person's projects, most recently touched first.
CREATE INDEX ix_projects_user ON projects (user_id, updated_at);

-- Which surface this transcript is. 'chat' or 'work'.
--
-- The conversation owns it, and a project only supplies the default at the
-- moment one is created there. One column decides what a transcript is, so
-- the rail, the composer and the gateway cannot disagree about it — and a
-- conversation does not change character later because the project it sits
-- in was edited.
ALTER TABLE conversations ADD COLUMN mode TEXT NOT NULL DEFAULT 'chat';

-- Null for the great majority of conversations, which belong to no project.
-- ON DELETE SET NULL rather than CASCADE: deleting a project is a decision
-- about the project, and taking a year of transcripts with it is not what
-- anybody means by it.
ALTER TABLE conversations ADD COLUMN project_id TEXT REFERENCES projects (id) ON DELETE SET NULL;

CREATE INDEX ix_conversations_project ON conversations (project_id, updated_at);
