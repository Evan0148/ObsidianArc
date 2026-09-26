-- Site login background images.
--
-- Four possible variants: landscape_light, landscape_dark, portrait_light, portrait_dark.
-- Binary blobs stored directly in the database rather than in settings JSON or disk,
-- consistent with wallpaper and attachments.
CREATE TABLE login_backgrounds (
    variant    TEXT PRIMARY KEY,
    mime       TEXT NOT NULL,
    data       %BLOB% NOT NULL,
    updated_at BIGINT NOT NULL
);
