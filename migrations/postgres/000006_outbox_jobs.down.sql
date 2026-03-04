-- Reverts 000006_outbox_jobs.up.sql
ALTER TABLE outbox_events DROP COLUMN IF EXISTS processor_id;
ALTER TABLE outbox_events DROP COLUMN IF EXISTS locked_until;
