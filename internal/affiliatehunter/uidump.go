//go:build parked

package affiliatehunter

import (
	"encoding/xml"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Bounds là hình chữ nhật [x1,y1][x2,y2] của một node UI.
type Bounds struct{ X1, Y1, X2, Y2 int }

// Center trả tâm điểm để Tap.
func (b Bounds) Center() (int, int) { return (b.X1 + b.X2) / 2, (b.Y1 + b.Y2) / 2 }

// UINode là một node trong cây uiautomator dump.
type UINode struct {
	Text        string
	ResourceID  string
	Class       string
	Package     string
	ContentDesc string
	Clickable   bool
	Scrollable  bool
	Bounds      Bounds
}

// UINodes là tập node kèm hàm tìm kiếm.
type UINodes []UINode

var boundsRe = regexp.MustCompile(`\[(\d+),(\d+)\]\[(\d+),(\d+)\]`)

func parseBounds(s string) Bounds {
	m := boundsRe.FindStringSubmatch(s)
	if m == nil {
		return Bounds{}
	}
	nums := make([]int, 4)
	for i := 0; i < 4; i++ {
		nums[i], _ = strconv.Atoi(m[i+1])
	}
	return Bounds{nums[0], nums[1], nums[2], nums[3]}
}

type xmlNode struct {
	XMLName     xml.Name  `xml:"node"`
	Text        string    `xml:"text,attr"`
	ResourceID  string    `xml:"resource-id,attr"`
	Class       string    `xml:"class,attr"`
	Package     string    `xml:"package,attr"`
	ContentDesc string    `xml:"content-desc,attr"`
	Clickable   string    `xml:"clickable,attr"`
	Scrollable  string    `xml:"scrollable,attr"`
	Bounds      string    `xml:"bounds,attr"`
	Children    []xmlNode `xml:"node"`
}

func (x xmlNode) toUINode() UINode {
	return UINode{
		Text:        x.Text,
		ResourceID:  x.ResourceID,
		Class:       x.Class,
		Package:     x.Package,
		ContentDesc: x.ContentDesc,
		Clickable:   x.Clickable == "true",
		Scrollable:  x.Scrollable == "true",
		Bounds:      parseBounds(x.Bounds),
	}
}

func flatten(x xmlNode, out *UINodes) {
	*out = append(*out, x.toUINode())
	for _, c := range x.Children {
		flatten(c, out)
	}
}

// ParseUIDump parse XML từ `uiautomator dump` thành danh sách node phẳng.
func ParseUIDump(dump string) (UINodes, error) {
	var root struct {
		XMLName xml.Name  `xml:"hierarchy"`
		Nodes   []xmlNode `xml:"node"`
	}
	dec := xml.NewDecoder(strings.NewReader(dump))
	dec.Strict = false
	if err := dec.Decode(&root); err != nil {
		return nil, fmt.Errorf("parse ui dump: %w", err)
	}
	var out UINodes
	for _, n := range root.Nodes {
		flatten(n, &out)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("ui dump rỗng")
	}
	return out, nil
}

// FindTextContains tìm node có text chứa s (không phân biệt hoa thường).
func (ns UINodes) FindTextContains(s string) UINodes {
	s = strings.ToLower(s)
	var out UINodes
	for _, n := range ns {
		if strings.Contains(strings.ToLower(n.Text), s) {
			out = append(out, n)
		}
	}
	return out
}

// FindByResourceID tìm node theo resource-id (chứa s để khỏi cứng package prefix).
func (ns UINodes) FindByResourceID(s string) UINodes {
	var out UINodes
	for _, n := range ns {
		if strings.Contains(n.ResourceID, s) {
			out = append(out, n)
		}
	}
	return out
}

// FindClickable tìm node clickable có text chứa s.
func (ns UINodes) FindClickable(s string) UINodes {
	var out UINodes
	for _, n := range ns.FindTextContains(s) {
		if n.Clickable {
			out = append(out, n)
		}
	}
	return out
}

// Texts trả mọi text không rỗng (dùng khi discovery / debug).
func (ns UINodes) Texts() []string {
	var out []string
	seen := map[string]bool{}
	for _, n := range ns {
		t := strings.TrimSpace(n.Text)
		if t != "" && !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	return out
}
