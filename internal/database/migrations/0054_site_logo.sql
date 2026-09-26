-- Site logo image for header brand, browser favicon, and PWA icon.
-- Stored as binary blob in database, consistent with wallpaper and login backgrounds.
CREATE TABLE site_logo (
    id         TEXT PRIMARY KEY,
    mime       TEXT NOT NULL,
    data       %BLOB% NOT NULL,
    updated_at BIGINT NOT NULL
);
