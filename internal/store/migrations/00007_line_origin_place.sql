-- +goose Up
-- A saved line's or trip's usual starting point: walking times use it when the request carries no location.
ALTER TABLE saved_lines ADD COLUMN origin_place_id INTEGER REFERENCES saved_places (id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE saved_lines DROP COLUMN origin_place_id;
