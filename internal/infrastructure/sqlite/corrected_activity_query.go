package sqlite

// Keep the immutable audit ledger separate from the economic timeline. A fix
// replaces the original at its original position, including fixes of fixes.
// Ordinary undo records retain their existing effective dates. This also reads
// legacy corrections correctly without rewriting their recorded timestamps.
const correctedActivityCTE = `WITH RECURSIVE correction_times(id, effective_at, effective_local_date, created_at, ordering_id) AS (
 SELECT a.id, a.effective_at, a.effective_local_date, a.created_at, a.id
 FROM activities a
 WHERE NOT EXISTS (SELECT 1 FROM activity_correction_groups g WHERE g.replacement_activity_id = a.id)
 UNION ALL
 SELECT g.replacement_activity_id, t.effective_at, t.effective_local_date, t.created_at, t.ordering_id
 FROM correction_times t JOIN activity_correction_groups g ON g.original_activity_id = t.id
), economic_activities AS (
 SELECT a.id, a.household_id, a.kind, a.reason, t.effective_at, t.effective_local_date,
        t.created_at, a.note, a.reverses_activity_id, a.correction_group_id, a.transaction_fx_rate, t.ordering_id
 FROM activities a JOIN correction_times t ON t.id = a.id
 WHERE NOT (a.correction_group_id IS NOT NULL AND a.reverses_activity_id IS NOT NULL)
 AND NOT EXISTS (SELECT 1 FROM activity_correction_groups g WHERE g.original_activity_id = a.id)
) `
