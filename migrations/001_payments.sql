-- One row per attempt to pay an invoice through bob's node.
CREATE TABLE payments (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,

    -- Read from the invoice text before we call the node (rule 3).
    payment_hash   BYTEA  NOT NULL CHECK (octet_length(payment_hash) = 32),
    invoice        TEXT   NOT NULL,

    -- Whole satoshis. Never a decimal type for money.
    amount_sat     BIGINT NOT NULL CHECK (amount_sat > 0),
    fee_limit_sat  BIGINT NOT NULL CHECK (fee_limit_sat >= 0),

    status         TEXT   NOT NULL DEFAULT 'PENDING'
                   CHECK (status IN ('PENDING', 'IN_FLIGHT', 'SUCCEEDED', 'FAILED')),

    -- The receipt. Present only when the node said "succeeded".
    preimage       BYTEA  CHECK (preimage IS NULL OR octet_length(preimage) = 32),
    failure_reason TEXT,

    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- A payment is SUCCEEDED if and only if we hold its receipt.
    CHECK ((status = 'SUCCEEDED') = (preimage IS NOT NULL))
);

-- Rule 4: at most one live attempt per invoice.
-- FAILED rows are left out, so a failed payment can be tried again.
CREATE UNIQUE INDEX one_live_payment_per_hash
    ON payments (payment_hash)
    WHERE status <> 'FAILED';

-- The tracker and the "oldest stuck payment" alert look up unfinished rows.
CREATE INDEX payments_unfinished
    ON payments (created_at)
    WHERE status IN ('PENDING', 'IN_FLIGHT');
