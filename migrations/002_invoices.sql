-- One row per invoice created on the receiving node (alice).
CREATE TABLE invoices (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    payment_hash  BYTEA  NOT NULL UNIQUE CHECK (octet_length(payment_hash) = 32),
    invoice       TEXT   NOT NULL,           -- the lnbc... text we hand to the payer
    amount_sat    BIGINT NOT NULL CHECK (amount_sat > 0),
    memo          TEXT   NOT NULL DEFAULT '',

    status        TEXT   NOT NULL DEFAULT 'OPEN'
                  CHECK (status IN ('OPEN', 'PAID', 'EXPIRED')),

    expires_at    TIMESTAMPTZ NOT NULL,      -- we choose it, so the sweeper never has to ask the node
    paid_at       TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),

    CHECK ((status = 'PAID') = (paid_at IS NOT NULL))
);

-- The sweeper looks for open invoices past their time.
CREATE INDEX invoices_open ON invoices (expires_at) WHERE status = 'OPEN';

-- Our place in the node's list of paid invoices. Exactly one row.
CREATE TABLE listener_state (
    only_row      BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (only_row),
    settle_index  BIGINT NOT NULL DEFAULT 0
);
INSERT INTO listener_state DEFAULT VALUES;
