package automation

import (
	"context"
	"fmt"
	"github.com/ninhlee99/ai-creator-os/internal/growth"
	"github.com/ninhlee99/ai-creator-os/internal/network"
	"time"
)

func (s *Service) produceAccount(ctx context.Context, a *network.Account) []string {
	var notes []string
	if _, err := s.Growth.EnsureProfile(a.ID); err != nil {
		return notes
	}
	today := s.today()
	loc := s.location()
	staleCutoff := time.Now().In(loc).AddDate(0, 0, -s.StaleItemDays).Format("2006-01-02")
	since := time.Now().In(loc).AddDate(0, 0, -s.DedupWindowDays).Format("2006-01-02")
	recents, _ := s.Growth.RecentNetworkItems(since, 500)

	due, err := s.Growth.PlannedItemsDue(a.ID, today, 60)
	if err != nil {
		return append(notes, a.Username+": lỗi đọc kế hoạch — "+err.Error())
	}
	made := 0
	for _, it := range due {
		if it.PlannedFor > today {
			continue
		}
		if it.PlannedFor < staleCutoff {
			_ = s.Growth.SetItemStatus(it.ID, growth.ItemDropped,
				fmt.Sprintf("Quá hạn %d ngày chưa sản xuất — hệ thống loại để không đăng dồn nội dung cũ", s.StaleItemDays))
			_ = s.Growth.Decide("growth_director", "drop_stale_item", a.Username,
				"mục quá hạn: "+it.Topic, map[string]any{"item": it.ID, "planned_for": it.PlannedFor})
			notes = append(notes, a.Username+": loại mục quá hạn \""+it.Topic+"\"")
			continue
		}
		if made >= s.MaxProductionsPerTick {
			continue
		}
		it = s.dedupCheck(a, it, recents)

		concept := growth.ConceptOf(it.FormatID, it.Topic)
		_ = s.Growth.SetItemConcept(it.ID, concept, growth.ConceptHash(concept))

		if s.dryRun() {
			notes = append(notes, a.Username+": DRY-RUN — chưa sản xuất thật cho \""+it.Topic+"\"")
			continue
		}
		p := s.producer()
		if p == nil {
			n, _ := s.Growth.NoteItemFailure(it.ID, "Chưa sản xuất được: backend sản xuất chưa được nối")
			if n == 1 {
				_ = s.Growth.InsertAlert(growth.Alert{
					AccountID: a.ID, Severity: "warn", Kind: "plan",
					Message:     "Chưa sản xuất được mục \"" + it.Topic + "\": backend sản xuất chưa được nối",
					ActionTaken: "Giữ mục ở trạng thái chờ và tự thử lại ở vòng sau",
				})
			}
			notes = append(notes, a.Username+": chưa sản xuất được \""+it.Topic+"\" — backend sản xuất chưa được nối")
			continue
		}
		claimed, err := s.Growth.ClaimItemForProduction(it.ID)
		if err != nil {
			notes = append(notes, a.Username+": lỗi khóa mục sản xuất \""+it.Topic+"\" — "+err.Error())
			continue
		}
		if !claimed {
			continue // another tick already owns or advanced this plan item
		}
		jobID, err := p.Enqueue(ctx, ProduceRequest{Account: a, Item: it})
		if err != nil {
			n, _ := s.Growth.NoteItemFailure(it.ID, "Chưa sản xuất được: "+err.Error())
			if n == 1 {
				_ = s.Growth.InsertAlert(growth.Alert{
					AccountID: a.ID, Severity: "warn", Kind: "plan",
					Message:     "Chưa sản xuất được mục \"" + it.Topic + "\": " + err.Error(),
					ActionTaken: "Giữ mục ở trạng thái chờ và tự thử lại ở vòng sau",
				})
			}
			notes = append(notes, a.Username+": chưa sản xuất được \""+it.Topic+"\" — "+err.Error())
			continue
		}
		title := growth.VariantTitle(it)
		if err := s.Growth.SetItemProduction(it.ID, jobID, title, growth.VariantCaption(it)); err != nil {
			notes = append(notes, a.Username+": lỗi ghi job cho \""+it.Topic+"\" — "+err.Error())
			continue
		}
		// The freshly-stamped concept joins this pass's comparison set so
		// a second account processed later in the same tick sees it.
		recents = append(recents, growth.PlanItem{
			ID: it.ID, AccountID: a.ID, FormatID: it.FormatID,
			VariantGroup: it.VariantGroup, ConceptText: concept,
		})
		made++
		notes = append(notes, a.Username+": đã xếp sản xuất \""+it.Topic+"\" ("+variantLabel(it.Variant)+", job "+jobID+")")
	}
	return notes
}

// dedupCheck runs the anti-duplicate guard for one due item. On a
// collision with another account's produced concept it forces a
// re-generation with a different angle — production continues, the script
// cannot be a near-copy (doc §3.1).
func (s *Service) dedupCheck(a *network.Account, it growth.PlanItem, recents []growth.PlanItem) growth.PlanItem {
	concept := growth.ConceptOf(it.FormatID, it.Topic)
	cand := growth.TokensOf(concept)
	var sets []map[string]bool
	for _, r := range recents {
		if r.ID == it.ID || growth.SameLine(it, r) {
			continue
		}
		sets = append(sets, growth.TokensOf(r.ConceptText))
	}
	hit, sim := growth.DecideDedup(cand, sets)
	if !hit {
		return it
	}
	newTopic := growth.ApplyAngle(it.Topic, growth.AngleFor(it.ID, it.Attempts))
	newConcept := growth.ConceptOf(it.FormatID, newTopic)
	if err := s.Growth.SetItemDedup(it.ID, newTopic, newConcept, growth.ConceptHash(newConcept)); err != nil {
		return it
	}
	it.Topic = newTopic
	_ = s.Growth.Decide("growth_director", "dedup_regenerate_angle", a.Username,
		fmt.Sprintf("concept trùng %.0f%% với nội dung đã sản xuất trên mạng — đổi góc triển khai", sim*100),
		map[string]any{"item": it.ID, "similarity": sim, "new_topic": newTopic})
	_ = s.Growth.InsertAlert(growth.Alert{
		AccountID: a.ID, Severity: "info", Kind: "dedup",
		Message:     fmt.Sprintf("Mục %q trùng ý tưởng (%.0f%%) với nội dung đã sản xuất trên mạng", concept, sim*100),
		ActionTaken: "Hệ thống tự đổi góc viết: \"" + newTopic + "\"",
	})
	return it
}

// -------------------------------------------------------- advance pass
