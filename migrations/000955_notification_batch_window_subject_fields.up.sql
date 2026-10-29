-- §35 (BUG-009): batched notifications were stored without body/action_url
-- because the window carried no subject context — aggregated rows could not
-- deep-link and did not say what was reacted to. Persist the caller's
-- subject title + deep link on the window so DispatchBatch can carry them
-- into the notifications row and the live WS payload.
ALTER TABLE notification_batch_windows
    ADD COLUMN subject_title text,
    ADD COLUMN action_url    text;
