-- +goose Up
-- A saved line can also be a trip between two stop areas, served by whatever
-- lines connect them (route_id and board_stop_id are empty for trips).
ALTER TABLE saved_lines ADD COLUMN kind TEXT NOT NULL DEFAULT 'line' CHECK (kind IN ('line', 'trip'));
ALTER TABLE saved_lines ADD COLUMN from_area_id TEXT NOT NULL DEFAULT '';
ALTER TABLE saved_lines ADD COLUMN to_area_id TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE saved_lines DROP COLUMN to_area_id;
ALTER TABLE saved_lines DROP COLUMN from_area_id;
ALTER TABLE saved_lines DROP COLUMN kind;
