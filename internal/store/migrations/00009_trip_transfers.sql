-- +goose Up
-- Where a trip changes: comma-separated stop areas, "alight" or "alight>board"
-- when the next ride leaves from another area a short walk away. Empty for
-- trips without changes.
ALTER TABLE saved_lines ADD COLUMN transfers TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE saved_lines DROP COLUMN transfers;
