package tiktok

import (
	"strings"
	"testing"
)

// Hand-computed test vector (cross-checked with openssl dgst -sha256 -hmac):
//
//	appSecret = "testsecret"
//	path      = "/api/affiliate/order/search"
//	params    = {app_key: TESTKEY123, shop_cipher: CIPHER456,
//	             sign: <excluded>, access_token: <excluded>}
//	body      = {"page_number":1,"page_size":50}   (compact JSON)
//	timestamp = 1700000000
//
// to_sign = "testsecret"
//   - "/api/affiliate/order/search"
//   - "app_keyTESTKEY123" + "shop_cipherCIPHER456" + "timestamp1700000000"
//   - `{"page_number":1,"page_size":50}`
//   - "testsecret"
const (
	signVectorHex = "dd8abdb85e724cedf82c552c7740848e66032b5665614b79f619a6b4926ae576"
	signVectorTS  = "1700000000"
)

func TestSignRequestVector(t *testing.T) {
	sig, ts := SignRequest(
		"testsecret",
		"/api/affiliate/order/search",
		map[string]string{
			"app_key":      "TESTKEY123",
			"shop_cipher":  "CIPHER456",
			"sign":         "SHOULD_BE_EXCLUDED",
			"access_token": "ALSO_EXCLUDED",
		},
		[]byte(`{"page_number":1,"page_size":50}`),
		1700000000,
	)
	if ts != signVectorTS {
		t.Fatalf("timestamp = %q, want %q", ts, signVectorTS)
	}
	if sig != signVectorHex {
		t.Fatalf("signature = %q, want %q", sig, signVectorHex)
	}
}

func TestSignRequestNoBody(t *testing.T) {
	// Same params, nil body must differ from the with-body vector.
	sig, _ := SignRequest("testsecret", "/api/affiliate/order/search",
		map[string]string{"app_key": "TESTKEY123"}, nil, 1700000000)
	if sig == signVectorHex {
		t.Fatal("nil-body signature must differ from body vector")
	}
	if len(sig) != 64 {
		t.Fatalf("signature not hex-sha256: %q", sig)
	}
}

func TestCallRefusesUnverifiedEndpoint(t *testing.T) {
	c := &ShopClient{AppKey: "k", AppSecret: "s", AccessToken: "t",
		HTTP: func(method, url string, headers map[string]string, body []byte) (int, []byte, error) {
			t.Fatal("must not fire HTTP for unverified endpoint")
			return 0, nil, nil
		}}
	for _, name := range []string{"affiliate_orders_search", "affiliate_open_collab_search", "no_such_endpoint"} {
		if _, err := c.Call(name, map[string]any{"page_number": 1}, nil); err == nil {
			t.Fatalf("Call(%q): expected refusal", name)
		} else if !strings.Contains(err.Error(), "no verified path") {
			t.Fatalf("Call(%q): wrong error: %v", name, err)
		}
	}
	// Wrapper inherits the fail-closed behavior.
	if _, err := c.AffiliateOrders(1700000000, 1700086400, 1, 50); err == nil {
		t.Fatal("AffiliateOrders: expected refusal")
	}
	if _, err := c.SearchOpenCollabProducts("lamp", "", 0, 1, 50); err == nil {
		t.Fatal("SearchOpenCollabProducts: expected refusal")
	}
}

func TestCallFiresVerifiedEndpoint(t *testing.T) {
	// Temporarily verify one endpoint path, then restore.
	old, ok := Endpoints["affiliate_orders_search"]
	Endpoints["affiliate_orders_search"] = "/api/affiliate/order/search"
	defer func() {
		if ok {
			Endpoints["affiliate_orders_search"] = old
		} else {
			delete(Endpoints, "affiliate_orders_search")
		}
	}()

	var sawURL string
	var sawHeaders map[string]string
	c := &ShopClient{AppKey: "APPK", AppSecret: "testsecret", AccessToken: "tok123", ShopCipher: "ciph",
		HTTP: func(method, url string, headers map[string]string, body []byte) (int, []byte, error) {
			sawURL, sawHeaders = url, headers
			return 200, []byte(`{"code":0,"message":"ok","data":{"orders":[]}}`), nil
		}}
	data, err := c.AffiliateOrders(1700000000, 1700086400, 1, 50)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if _, ok := data["orders"]; !ok {
		t.Fatalf("data = %v", data)
	}
	for _, want := range []string{"app_key=APPK", "shop_cipher=ciph", "timestamp=", "sign=", "start_time=1700000000"} {
		if !strings.Contains(sawURL, want) {
			t.Fatalf("url missing %q: %s", want, sawURL)
		}
	}
	if sawHeaders["x-tts-access-token"] != "tok123" {
		t.Fatalf("access token header = %v", sawHeaders)
	}
}

func TestCallShopAPIError(t *testing.T) {
	old := Endpoints["showcase_list"]
	Endpoints["showcase_list"] = "/api/showcase/list"
	defer func() { Endpoints["showcase_list"] = old }()

	c := &ShopClient{AppKey: "k", AppSecret: "s", AccessToken: "t",
		HTTP: func(method, url string, headers map[string]string, body []byte) (int, []byte, error) {
			return 200, []byte(`{"code":40001,"message":"sign error"}`), nil
		}}
	if _, err := c.Call("showcase_list", nil, nil); err == nil {
		t.Fatal("expected shop API error")
	} else if !strings.Contains(err.Error(), "40001") {
		t.Fatalf("wrong error: %v", err)
	}
}
