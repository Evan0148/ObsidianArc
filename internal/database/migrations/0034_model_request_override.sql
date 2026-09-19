-- Generic API request override for models: allows operators to supply extra
-- body parameters (e.g. {"reasoning_effort": "low"}) that merge into the outbound request.
ALTER TABLE models ADD COLUMN request_override TEXT NOT NULL DEFAULT '{}';

-- Ensure all models are granted 'use' access to all user groups by default.
INSERT INTO group_models (group_id, model_id, access)
SELECT g.id, m.id, 'use'
FROM user_groups g
CROSS JOIN models m
WHERE NOT EXISTS (
    SELECT 1 FROM group_models gm
    WHERE gm.group_id = g.id AND gm.model_id = m.id
);

UPDATE group_models SET access = 'use';
