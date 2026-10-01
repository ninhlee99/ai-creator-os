package tiktok

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"time"
)

// ShopBase is the TikTok Shop Open Platform host.
const ShopBase = "https://open-api.tiktokglobalshop.com"

// Endpoints maps a logical name to its versioned path. Empty string = NOT
// yet verified in the Partner Center docv2 sandbox. Fill these in after
// probing candidates with sandbox keys; Call refuses to fire an unverified
// path (fail closed), mirroring the Python client's philosophy.
var Endpoints = map[string]string{
	// Hunter: search Affiliate Product Marketplace (open collaboration)
	"affiliate_open_collab_search": "",
	// Hunter: product detail lookup by IDs
	"affiliate_open_collab_products_by_ids": "",
	// Content: manage creator showcase (up to 2000 products)
	"showcase_list": "",
	"showcase_add":  "",
	// Content: generate affiliate promotion links
	"affiliate_link_generate": "",
	// Analyst: affiliate orders = commission reconciliation feed
	"affiliate_orders_search": "",
	// Analyst: sample application tracking
	"sample_applications_search": "",
}

// SignRequest builds the TikTok Shop HMAC-SHA256 signature.
//
// Construction: app_secret + path + sorted(key+value, excluding "sign" and
// "access_token"; "timestamp" is part of the signed params) + compact-JSON
// body + app_secret, keyed with app_secret. Returns (signature, timestamp).
func SignRequest(appSecret, path string, queryParams map[string]string, body []byte, timestamp int64) (sig, ts string) {
	ts = strconv.FormatInt(timestamp, 10)
	params := make(map[string]string, len(queryParams)+1)
	for k, v := range queryParams {
		if k == "sign" || k == "access_token" {
			continue
		}
		params[k] = v
	}
	params["timestamp"] = ts

	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	flat := ""
	for _, k := range keys {
		flat += k + params[k]
	}

	base := path + flat
	if len(body) > 0 {
		base += string(body)
	}
	toSign := appSecret + base + appSecret
	mac := hmac.New(sha256.New, []byte(appSecret))
	mac.Write([]byte(toSign))
	return hex.EncodeToString(mac.Sum(nil)), ts
}

// ---------------------------------------------------------------------------
// ShopClient — port of tiktok/shop/client.py ShopClient.
//
// Auth: app key + app secret, HMAC-SHA256 signing, creator OAuth access
// token. The exact canonicalization MUST be re-verified against Partner
// Center docv2 in the sandbox before production use — a wrong guess fails
// closed (40001).
//
// Human steps (not code):
//  1. Register app at partner.tiktokshop.com (Affiliate developer type).
//  2. Creator authorizes via Seller Center / TikTok app -> access token.
//  3. App review before production (sandbox available meanwhile).
// ---------------------------------------------------------------------------

// ShopClient is a TikTok Shop Open Platform client.
type ShopClient struct {
	AppKey      string
	AppSecret   string
	AccessToken string
	ShopCipher  string
	// HTTP is the injectable transport; nil means DefaultHTTP.
	HTTP HTTPFunc
}

func (c *ShopClient) http() HTTPFunc {
	if c.HTTP != nil {
		return c.HTTP
	}
	return DefaultHTTP
}

// Call invokes a registered endpoint by logical name with signed query
// params. It refuses to fire an endpoint whose path is not yet verified.
func (c *ShopClient) Call(name string, params map[string]any, body map[string]any) (map[string]any, error) {
	path := Endpoints[name]
	if path == "" {
		return nil, fmt.Errorf("endpoint %q has no verified path yet — "+
			"confirm it in Partner Center docv2 sandbox, then fill "+
			"Endpoints in internal/tiktok/shop.go", name)
	}
	strParams := make(map[string]string, len(params)+3)
	for k, v := range params {
		strParams[k] = fmt.Sprintf("%v", v)
	}
	strParams["app_key"] = c.AppKey
	if c.ShopCipher != "" {
		strParams["shop_cipher"] = c.ShopCipher
	}
	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body) // compact separators, like Python's separators=(",", ":")
		if err != nil {
			return nil, err
		}
	}
	sig, ts := SignRequest(c.AppSecret, path, strParams, bodyBytes, time.Now().Unix())
	strParams["timestamp"] = ts
	strParams["sign"] = sig

	q := url.Values{}
	for k, v := range strParams {
		q.Set(k, v)
	}
	callURL := ShopBase + path + "?" + q.Encode()
	headers := map[string]string{
		"x-tts-access-token": c.AccessToken,
		"Content-Type":       "application/json",
	}
	_, raw, err := c.http()("GET", callURL, headers, bodyBytes)
	if err != nil {
		return nil, err
	}
	resp := decodeJSON(raw)
	if code, ok := resp["code"].(float64); ok && code != 0 {
		return nil, fmt.Errorf("shop API error %.0f: %.300v", code, resp)
	}
	if data, ok := resp["data"].(map[string]any); ok {
		return data, nil
	}
	return resp, nil
}

// SearchOpenCollabProducts — Hunter: products on the Affiliate Marketplace
// open to collaboration.
func (c *ShopClient) SearchOpenCollabProducts(keyword, categoryID string, minCommissionRate, page, pageSize int) (map[string]any, error) {
	params := map[string]any{"page_number": page, "page_size": pageSize}
	if keyword != "" {
		params["keyword"] = keyword
	}
	if categoryID != "" {
		params["category_id"] = categoryID
	}
	if minCommissionRate != 0 {
		params["min_commission_rate"] = minCommissionRate
	}
	return c.Call("affiliate_open_collab_search", params, nil)
}

// AffiliateOrders — Analyst: creator affiliate orders (commission
// reconciliation feed).
func (c *ShopClient) AffiliateOrders(startTS, endTS int64, page, pageSize int) (map[string]any, error) {
	return c.Call("affiliate_orders_search", map[string]any{
		"start_time":  startTS,
		"end_time":    endTS,
		"page_number": page,
		"page_size":   pageSize,
	}, nil)
}
