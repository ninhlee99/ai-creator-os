// Package products is the product-discovery layer for affiliate autopilot.
//
// Each network account has a theme (chủ đề) chosen by Ninh. The system
// searches for HIGH-COMMISSION products matching that theme, ranks them,
// and feeds the winner's real listing photos into the studio pipeline as
// the product-lock ground truth. Ninh never hunts products by hand.
//
// Providers implement the Provider interface. The TikTok Shop provider is
// fail-closed until the Partner Center endpoints are verified and creator
// OAuth is connected (a human step) — see internal/tiktok/shop.go.
package products
