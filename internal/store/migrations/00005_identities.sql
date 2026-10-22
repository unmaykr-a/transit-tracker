-- +goose Up
-- External sign-in identities (OIDC): one row per issuer + subject, linked to a local user.
CREATE TABLE user_identities (
    issuer      TEXT NOT NULL,
    subject     TEXT NOT NULL,
    user_id     INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    email       TEXT NOT NULL DEFAULT '',
    created_at  INTEGER NOT NULL,
    last_login  INTEGER NOT NULL,
    PRIMARY KEY (issuer, subject)
);
CREATE INDEX user_identities_user_id ON user_identities (user_id);

-- +goose Down
DROP TABLE user_identities;
