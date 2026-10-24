-- +goose Up
-- The national timetable's route ids behind each route (comma-separated), so
-- departures from it find the right route when lines of different regions share
-- a number (Rapla's bus 5, Tallinn's bus 5).
ALTER TABLE routes ADD COLUMN otp_ids TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE routes DROP COLUMN otp_ids;
