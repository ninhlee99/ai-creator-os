package growth

import (
	"testing"
	"time"
)

func draft(date, format, variant, topic string) PlanDraft {
	return PlanDraft{
		Date: date, FormatID: format, Variant: variant, Topic: topic,
		Hook: "hook", Group: date + "|" + format,
	}
}

func TestPlanItemProductionTrail(t *testing.T) {
	st := testStore(t)
	today := time.Now().Format("2006-01-02")
	if _, err := st.EnsureProfile(7, "tiktok", true); err != nil {
		t.Fatal(err)
	}
	_, err := st.InsertPlan(7, "d30", "test", []PlanDraft{
		draft(today, "truyen_ma", VariantTikTok, "truyện ma — tập 1"),
		draft(today, "truyen_ma", VariantShorts, "truyện ma — tập 1"),
		draft(today, "truyen_ma", VariantYouTube, "truyện ma — bản dài tập 1"),
	})
	if err != nil {
		t.Fatal(err)
	}

	due, err := st.PlannedItemsDue(7, today, 60)
	if err != nil || len(due) != 3 {
		t.Fatalf("due = %d items, err %v; want 3", len(due), err)
	}
	if due[0].VariantGroup != today+"|truyen_ma" {
		t.Errorf("variant group not stored: %q", due[0].VariantGroup)
	}
	// Yesterday's items are due; tomorrow's are not.
	if _, err := st.InsertPlan(8, "d30", "test", []PlanDraft{
		draft("2999-01-01", "x", VariantTikTok, "xa"),
	}); err != nil {
		t.Fatal(err)
	}
	if future, _ := st.PlannedItemsDue(8, today, 60); len(future) != 0 {
		t.Errorf("future items listed as due: %+v", future)
	}

	it := due[0]
	if err := st.SetItemConcept(it.ID, "truyen_ma truyện ma", ConceptHash("truyen_ma truyện ma")); err != nil {
		t.Fatal(err)
	}
	if err := st.SetItemProduction(it.ID, "job-1", "title", "caption"); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetItem(it.ID)
	if err != nil || got == nil {
		t.Fatalf("GetItem: %v %+v", err, got)
	}
	if got.Status != ItemProducing || got.StudioJobID != "job-1" || got.PubTitle != "title" {
		t.Errorf("after production link: %+v", got)
	}
	if err := st.SetItemProduced(it.ID, "xong"); err != nil {
		t.Fatal(err)
	}
	got, _ = st.GetItem(it.ID)
	if got.Status != ItemProduced || got.ProducedAt == "" {
		t.Errorf("after produced: %+v", got)
	}

	// The produced item is in the network dedup set; planned ones are not.
	recent, err := st.RecentNetworkItems("2000-01-01", 50)
	if err != nil || len(recent) != 1 || recent[0].ID != it.ID {
		t.Errorf("recent = %+v err %v, want only the produced item", recent, err)
	}

	if err := st.SetItemPublished(it.ID, "VID123"); err != nil {
		t.Fatal(err)
	}
	got, _ = st.GetItem(it.ID)
	if got.Status != ItemPublished || got.PublishedRef != "VID123" {
		t.Errorf("after publish: %+v", got)
	}

	// Related links land both ways.
	other := due[1]
	if err := st.LinkRelatedItems(it.ID, other.ID); err != nil {
		t.Fatal(err)
	}
	got, _ = st.GetItem(it.ID)
	otherGot, _ := st.GetItem(other.ID)
	if got.RelatedItemID != other.ID || otherGot.RelatedItemID != it.ID {
		t.Errorf("related link not bidirectional: %d <-> %d", got.RelatedItemID, otherGot.RelatedItemID)
	}

	// A failed enqueue keeps the item planned and counts attempts.
	n, err := st.NoteItemFailure(other.ID, "boom")
	if err != nil || n != 1 {
		t.Errorf("NoteItemFailure = %d, %v; want 1", n, err)
	}
	otherGot, _ = st.GetItem(other.ID)
	if otherGot.Status != ItemPlanned || otherGot.PubNote != "boom" {
		t.Errorf("after failed attempt: %+v", otherGot)
	}

	// ItemsByStatus sees the in-flight set.
	inFlight, err := st.ItemsByStatus(7, []string{ItemPublished, ItemPlanned}, 50)
	if err != nil || len(inFlight) != 3 {
		t.Errorf("in-flight = %d (%v), want 3", len(inFlight), err)
	}
}

func TestSetItemDedup(t *testing.T) {
	st := testStore(t)
	today := time.Now().Format("2006-01-02")
	if _, err := st.InsertPlan(9, "d30", "test", []PlanDraft{
		draft(today, "truyen_ma", VariantTikTok, "truyện ma — tập 1"),
	}); err != nil {
		t.Fatal(err)
	}
	due, _ := st.PlannedItemsDue(9, today, 10)
	if len(due) != 1 {
		t.Fatalf("due = %d, want 1", len(due))
	}
	nc := ConceptOf("truyen_ma", "truyện ma — hậu trường ít người biết")
	if err := st.SetItemDedup(due[0].ID, "truyện ma — hậu trường ít người biết", nc, ConceptHash(nc)); err != nil {
		t.Fatal(err)
	}
	got, _ := st.GetItem(due[0].ID)
	if got.DedupAction != "angle_regenerated" || got.Topic != "truyện ma — hậu trường ít người biết" {
		t.Errorf("dedup rewrite not stored: %+v", got)
	}
	if got.ConceptHash != ConceptHash(nc) {
		t.Errorf("concept hash not updated: %q", got.ConceptHash)
	}
}
