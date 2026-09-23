-- Whether a turn's token figures were counted by the provider or estimated
-- here.
--
-- Some gateways in front of web products answer without ever saying what the
-- answer cost. Those turns used to be recorded as zero tokens, which is a
-- claim that nothing was read or written; they are now estimated from the
-- text instead, and this column is what keeps the estimate from being
-- mistaken for a count. Every row written before it existed was a count or a
-- zero, so FALSE is the truthful default for all of them.
ALTER TABLE usage_records ADD COLUMN usage_estimated BOOLEAN NOT NULL DEFAULT FALSE;
