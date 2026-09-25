-- Where and in what browser a session proved its code for the backoffice.
--
-- Checked on every request when the operator asks for it: a visit unlocked
-- from one network, or one browser, is not taken to still be the same person
-- once the requests start arriving from another — the shape a copied
-- session cookie has.
ALTER TABLE sessions ADD COLUMN backoffice_ip TEXT NOT NULL DEFAULT '';
ALTER TABLE sessions ADD COLUMN backoffice_ua TEXT NOT NULL DEFAULT '';
