-- v0.1.4 schema 6: persist Starting Point and reconciliation costs.
-- This migration is additive; cost basis remains a derived read model.

ALTER TABLE history_origin_components
    ADD COLUMN unit_cost TEXT;

ALTER TABLE activity_effects
    ADD COLUMN cost_unit_price TEXT;
