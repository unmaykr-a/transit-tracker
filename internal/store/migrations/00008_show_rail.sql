-- +goose Up
-- Railway lines on the live map are opt-in.
ALTER TABLE user_prefs ADD COLUMN show_rail INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE user_prefs DROP COLUMN show_rail;
