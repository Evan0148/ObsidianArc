-- Whether a group's members may open the terminal.
--
-- The terminal used to be a backoffice section, so the only accounts that
-- could reach it were administrators. It is now in every account's menu,
-- where it runs the commands that account could already run by clicking —
-- its own settings, keys, conversations, usage and export — and nothing
-- more, because every command goes through the same endpoints and the same
-- checks the screens do.
--
-- Defaults to true, as the other group capabilities do: the column is a way
-- to take the terminal away from a group, not a new thing each group has to
-- be given. Administrators keep it whatever their group says.

ALTER TABLE user_groups ADD COLUMN allow_terminal BOOLEAN NOT NULL DEFAULT TRUE;
