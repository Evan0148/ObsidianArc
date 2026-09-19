-- Whether members of this group see their group membership expiry date in the usage drawer.
ALTER TABLE user_groups ADD COLUMN show_expiry BOOLEAN NOT NULL DEFAULT TRUE;
