package affiliatehunter

import (
	"strings"
	"testing"
)

const sampleDump = `<?xml version='1.0' encoding='UTF-8' standalone='yes' ?>
<hierarchy rotation="0">
  <node index="0" text="" resource-id="" class="android.widget.FrameLayout" package="com.zhiliaoapp.musically" content-desc="" checkable="false" checked="false" clickable="false" enabled="true" focusable="false" focused="false" scrollable="false" long-clickable="false" password="false" selected="false" bounds="[0,0][1080,2400]">
    <node index="1" text="Product Marketplace" resource-id="com.zhiliaoapp.musically:id/title" class="android.widget.TextView" package="com.zhiliaoapp.musically" content-desc="" clickable="false" scrollable="false" bounds="[0,100][1080,200]" />
    <node index="2" text="Váy hoa nhí" resource-id="com.zhiliaoapp.musically:id/product_name" class="android.widget.TextView" package="com.zhiliaoapp.musically" content-desc="" clickable="true" scrollable="false" bounds="[0,300][540,900]" />
    <node index="3" text="Hoa hồng 15%" resource-id="com.zhiliaoapp.musically:id/commission" class="android.widget.TextView" package="com.zhiliaoapp.musically" content-desc="" clickable="false" scrollable="false" bounds="[0,900][540,960]" />
    <node index="4" text="" resource-id="com.zhiliaoapp.musically:id/list" class="androidx.recyclerview.widget.RecyclerView" package="com.zhiliaoapp.musically" content-desc="" clickable="false" scrollable="true" bounds="[0,200][1080,2400]" />
  </node>
</hierarchy>`

func TestParseUIDump(t *testing.T) {
	ns, err := ParseUIDump(sampleDump)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(ns) != 5 {
		t.Fatalf("want 5 nodes, got %d", len(ns))
	}
}

func TestFindTextContains(t *testing.T) {
	ns, _ := ParseUIDump(sampleDump)
	got := ns.FindTextContains("hoa hồng")
	if len(got) != 1 || !strings.Contains(got[0].Text, "15%") {
		t.Fatalf("find commission: %+v", got)
	}
	// case-insensitive
	if len(ns.FindTextContains("MARKETPLACE")) != 1 {
		t.Fatal("case-insensitive search failed")
	}
}

func TestFindByResourceID(t *testing.T) {
	ns, _ := ParseUIDump(sampleDump)
	got := ns.FindByResourceID("product_name")
	if len(got) != 1 || got[0].Text != "Váy hoa nhí" {
		t.Fatalf("find by id: %+v", got)
	}
}

func TestFindClickable(t *testing.T) {
	ns, _ := ParseUIDump(sampleDump)
	got := ns.FindClickable("váy")
	if len(got) != 1 {
		t.Fatalf("want 1 clickable, got %d", len(got))
	}
	x, y := got[0].Bounds.Center()
	if x != 270 || y != 600 {
		t.Fatalf("center = %d,%d", x, y)
	}
}

func TestBoundsParse(t *testing.T) {
	b := parseBounds("[0,300][540,900]")
	if b.X2 != 540 || b.Y2 != 900 {
		t.Fatalf("bounds: %+v", b)
	}
	if parseBounds("nonsense") != (Bounds{}) {
		t.Fatal("bad bounds should be zero")
	}
}

func TestParseUIDumpBadInput(t *testing.T) {
	if _, err := ParseUIDump("not xml"); err == nil {
		t.Fatal("want error for bad xml")
	}
	if _, err := ParseUIDump(`<hierarchy></hierarchy>`); err == nil {
		t.Fatal("want error for empty dump")
	}
}
