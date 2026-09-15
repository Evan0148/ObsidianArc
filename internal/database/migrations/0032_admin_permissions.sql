-- Existing operators retain full control when page grants become explicit.
ALTER TABLE users ADD COLUMN admin_permissions TEXT NOT NULL DEFAULT '[]';
UPDATE users SET role = 'super_admin' WHERE role = 'admin';
