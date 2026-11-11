-- Switches an operator can flip while the service runs. Exactly one row.
CREATE TABLE settings (
    only_row        BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (only_row),
    payouts_paused  BOOLEAN NOT NULL DEFAULT FALSE,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO settings DEFAULT VALUES;
