-- §38: human-readable ticket reference. GENERATED ALWAYS AS IDENTITY
-- assigns sequential numbers to existing rows and every future insert;
-- routes render it as JC-<number> and accept it interchangeably with the
-- UUID (see GetTicketByNumber in queries/tickets.sql).

ALTER TABLE tickets
    ADD COLUMN IF NOT EXISTS ticket_number bigint GENERATED ALWAYS AS IDENTITY UNIQUE;
