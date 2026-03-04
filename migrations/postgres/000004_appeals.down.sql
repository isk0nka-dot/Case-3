-- Reverts 000004_appeals.up.sql -- Note: Requires extreme caution in production
-- We will only remove the trigger for the down migration, assuming the table existed
DROP TRIGGER IF EXISTS trg_update_appeals_updated_at ON appeals;
DROP FUNCTION IF EXISTS update_updated_at_column();
