package growth

import (
	"database/sql"
	"strings"
)

func (s *Store) InsertPlan(accountID int64, phase, rationale string, drafts []PlanDraft) (int64, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(
		`UPDATE content_plans SET status = 'superseded' WHERE account_id = ? AND status = 'active'`,
		accountID); err != nil {
		return 0, err
	}
	// Discard the superseded plan's unexecuted items so the new plan can
	// schedule the same (day, format, variant) slots — the UNIQUE index
	// guards the ACTIVE plan against duplicate scheduling, and published
	// history is kept untouched.
	if _, err := tx.Exec(
		`DELETE FROM content_plan_items
		 WHERE account_id = ? AND status = 'planned'
		   AND plan_id IN (SELECT id FROM content_plans WHERE account_id = ? AND status = 'superseded')`,
		accountID, accountID); err != nil {
		return 0, err
	}
	res, err := tx.Exec(
		`INSERT INTO content_plans (account_id, phase, rationale) VALUES (?, ?, ?)`,
		accountID, phase, rationale)
	if err != nil {
		return 0, err
	}
	planID, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	for _, d := range drafts {
		group := d.Group
		if group == "" {
			group = d.Date + "|" + d.FormatID
		}
		if _, err := tx.Exec(
			`INSERT OR IGNORE INTO content_plan_items
			 (plan_id, account_id, planned_for, format_id, platform_variant, topic, hook, series_ep, status, variant_group)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'planned', ?)`,
			planID, accountID, d.Date, d.FormatID, d.Variant, d.Topic, d.Hook, d.SeriesEp, group); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return planID, nil
}

// ActivePlan returns the current plan and its items (nil plan when none).
func (s *Store) ActivePlan(accountID int64) (*Plan, []PlanItem, error) {
	row := s.db.QueryRow(
		`SELECT id, account_id, phase, status, COALESCE(rationale,''), created_at
		 FROM content_plans WHERE account_id = ? AND status = 'active' ORDER BY id DESC LIMIT 1`, accountID)
	var p Plan
	err := row.Scan(&p.ID, &p.AccountID, &p.Phase, &p.Status, &p.Rationale, &p.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	items, err := s.planItems(p.ID)
	return &p, items, err
}

const planItemCols = `id, plan_id, account_id, planned_for, format_id, platform_variant,
	COALESCE(topic,''), COALESCE(hook,''), series_ep, status, COALESCE(content_item_id,0),
	COALESCE(published_ref,''), COALESCE(studio_job_id,''), COALESCE(concept_text,''),
	COALESCE(concept_hash,''), COALESCE(variant_group,''), COALESCE(dedup_action,''),
	attempts, COALESCE(pub_title,''), COALESCE(pub_caption,''), COALESCE(pub_note,''),
	COALESCE(related_item_id,0), COALESCE(produced_at,'')`

func scanPlanItem(rows *sql.Rows) (PlanItem, error) {
	var it PlanItem
	err := rows.Scan(&it.ID, &it.PlanID, &it.AccountID, &it.PlannedFor, &it.FormatID,
		&it.Variant, &it.Topic, &it.Hook, &it.SeriesEp, &it.Status, &it.ContentItemID,
		&it.PublishedRef, &it.StudioJobID, &it.ConceptText, &it.ConceptHash,
		&it.VariantGroup, &it.DedupAction, &it.Attempts, &it.PubTitle, &it.PubCaption,
		&it.PubNote, &it.RelatedItemID, &it.ProducedAt)
	return it, err
}

func (s *Store) planItems(planID int64) ([]PlanItem, error) {
	rows, err := s.db.Query(
		`SELECT `+planItemCols+` FROM content_plan_items WHERE plan_id = ? ORDER BY planned_for, id`, planID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PlanItem
	for rows.Next() {
		it, err := scanPlanItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// UpcomingItems returns the next planned items from today (inclusive).
func (s *Store) UpcomingItems(accountID int64, today string, limit int) ([]PlanItem, error) {
	_, items, err := s.ActivePlan(accountID)
	if err != nil {
		return nil, err
	}
	var out []PlanItem
	for _, it := range items {
		if it.Status == ItemPlanned && it.PlannedFor >= today {
			out = append(out, it)
			if len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}

// --------------------------------------------------- production (ph. 2)

// ActivePlanItems returns every item of the account's active plan (any
// status), oldest scheduled first.
func (s *Store) ActivePlanItems(accountID int64) ([]PlanItem, error) {
	_, items, err := s.ActivePlan(accountID)
	return items, err
}

// PlannedItemsDue returns still-planned items scheduled on or before
// today (the caller applies the per-variant schedule lag and caps).
func (s *Store) PlannedItemsDue(accountID int64, today string, limit int) ([]PlanItem, error) {
	rows, err := s.db.Query(
		`SELECT `+planItemCols+` FROM content_plan_items
		 WHERE account_id = ? AND status = 'planned' AND planned_for <= ?
		 ORDER BY planned_for, id LIMIT ?`, accountID, today, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PlanItem
	for rows.Next() {
		it, err := scanPlanItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// ItemsByStatus returns the account's items in any of the given statuses,
// across all plans (production history included), newest activity last.
func (s *Store) ItemsByStatus(accountID int64, statuses []string, limit int) ([]PlanItem, error) {
	if len(statuses) == 0 {
		return nil, nil
	}
	ph := make([]string, len(statuses))
	args := []any{accountID}
	for i, st := range statuses {
		ph[i] = "?"
		args = append(args, st)
	}
	args = append(args, limit)
	rows, err := s.db.Query(
		`SELECT `+planItemCols+` FROM content_plan_items
		 WHERE account_id = ? AND status IN (`+strings.Join(ph, ",")+`)
		 ORDER BY planned_for, id LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PlanItem
	for rows.Next() {
		it, err := scanPlanItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// GetItem returns one plan item (nil when absent).
func (s *Store) GetItem(id int64) (*PlanItem, error) {
	rows, err := s.db.Query(`SELECT `+planItemCols+` FROM content_plan_items WHERE id = ?`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, rows.Err()
	}
	it, err := scanPlanItem(rows)
	if err != nil {
		return nil, err
	}
	return &it, nil
}

// SetItemConcept stamps the dedup fingerprint on an item.
func (s *Store) SetItemConcept(id int64, conceptText, conceptHash string) error {
	_, err := s.db.Exec(
		`UPDATE content_plan_items SET concept_text = ?, concept_hash = ? WHERE id = ?`,
		conceptText, conceptHash, id)
	return err
}

// SetItemDedup records a forced re-generation: new topic + concept and the
// dedup_action marker for the UI lineage display.
func (s *Store) SetItemDedup(id int64, topic, conceptText, conceptHash string) error {
	_, err := s.db.Exec(
		`UPDATE content_plan_items SET topic = ?, concept_text = ?, concept_hash = ?,
		   dedup_action = 'angle_regenerated' WHERE id = ?`,
		topic, conceptText, conceptHash, id)
	return err
}

// SetItemProduction links the Studio job and the per-variant publish
// metadata, moving the item to producing.
func (s *Store) SetItemProduction(id int64, jobID, pubTitle, pubCaption string) error {
	_, err := s.db.Exec(
		`UPDATE content_plan_items SET status = 'producing', studio_job_id = ?,
		   pub_title = ?, pub_caption = ? WHERE id = ?`,
		jobID, pubTitle, pubCaption, id)
	return err
}

// NoteItemFailure records a failed enqueue attempt (item stays planned and
// will be retried next tick). Returns the new attempt count.
func (s *Store) NoteItemFailure(id int64, note string) (int, error) {
	if _, err := s.db.Exec(
		`UPDATE content_plan_items SET attempts = attempts + 1, pub_note = ? WHERE id = ?`,
		note, id); err != nil {
		return 0, err
	}
	it, err := s.GetItem(id)
	if err != nil || it == nil {
		return 0, err
	}
	return it.Attempts, nil
}

// SetItemStatus moves an item to a status with an honest state note.
func (s *Store) SetItemStatus(id int64, status, note string) error {
	_, err := s.db.Exec(
		`UPDATE content_plan_items SET status = ?, pub_note = ? WHERE id = ?`,
		status, note, id)
	return err
}

// ClaimItemForPublishing atomically reserves one rendered item for upload.
// Only one Growth tick can move an eligible item into the in-flight state.
func (s *Store) ClaimItemForPublishing(id int64) (bool, error) {
	res, err := s.db.Exec(
		`UPDATE content_plan_items SET status = 'publishing', pub_note = ?
		 WHERE id = ? AND status IN ('produced', 'waiting_connect', 'waiting_quota')`,
		"Đang gửi video lên YouTube.", id)
	if err != nil {
		return false, err
	}
	changed, err := res.RowsAffected()
	return changed == 1, err
}

// RecoverInterruptedPublishingItems moves uploads left in flight by a
// process restart to review. Provider may have accepted the request.
func (s *Store) RecoverInterruptedPublishingItems() (int64, error) {
	res, err := s.db.Exec(
		`UPDATE content_plan_items SET status = 'needs_review', pub_note = ? WHERE status = 'publishing'`,
		"Growth upload bị gián đoạn khi ứng dụng dừng; kiểm tra YouTube Studio trước khi xử lý tiếp.")
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// SetItemProduced marks the render finished (produced_at stamped).
func (s *Store) SetItemProduced(id int64, note string) error {
	_, err := s.db.Exec(
		`UPDATE content_plan_items SET status = 'produced', produced_at = ?, pub_note = ? WHERE id = ?`,
		nowStr(), note, id)
	return err
}

// SetItemPublished records the platform video id after a real upload.
func (s *Store) SetItemPublished(id int64, ref string) error {
	_, err := s.db.Exec(
		`UPDATE content_plan_items SET status = 'published', published_ref = ?, pub_note = '' WHERE id = ?`,
		ref, id)
	return err
}

// LinkRelatedItems records the sibling relation (Short <-> long-form)
// both ways.
func (s *Store) LinkRelatedItems(aID, bID int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE content_plan_items SET related_item_id = ? WHERE id = ?`, bID, aID); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE content_plan_items SET related_item_id = ? WHERE id = ?`, aID, bID); err != nil {
		return err
	}
	return tx.Commit()
}

// RecentNetworkItems returns recently produced/published items across the
// whole network (the dedup guard's comparison set). produced_at falls back
// to the plan date for items still rendering.
