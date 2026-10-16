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

// Review facts only consume holding quantities and holding cost events. Find
// the correction chains that can contain either kind of effect first, then
// replay corrections only for those chains. Without this scope the recursive
// timeline starts at every cash activity in a household even though review
// rules never inspect account cash.
const reviewFactActivityCTE = `WITH RECURSIVE relevant_activity_ids(id) AS (
 SELECT a.id FROM activities a
 JOIN activity_effects e ON e.activity_id = a.id
 WHERE a.household_id = ? AND e.target IN ('holding_quantity','holding_cost')
 UNION
 SELECT g.original_activity_id
 FROM activity_correction_groups g
 JOIN relevant_activity_ids relevant ON relevant.id = g.replacement_activity_id
), correction_times(id, effective_at, effective_local_date, created_at, ordering_id) AS (
 SELECT a.id, a.effective_at, a.effective_local_date, a.created_at, a.id
 FROM activities a JOIN relevant_activity_ids relevant ON relevant.id = a.id
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
