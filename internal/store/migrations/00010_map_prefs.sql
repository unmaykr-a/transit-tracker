-- +goose Up
-- Map display preferences: vehicle markers as line pills or top-down vehicles,
-- and whether markers are predicted ahead of the last position (on by default).
ALTER TABLE user_prefs ADD COLUMN vehicle_icons TEXT NOT NULL DEFAULT 'pill';
ALTER TABLE user_prefs ADD COLUMN predict_positions INTEGER NOT NULL DEFAULT 1;

-- +goose Down
ALTER TABLE user_prefs DROP COLUMN predict_positions;
ALTER TABLE user_prefs DROP COLUMN vehicle_icons;
