"""TikTok Shop Open Platform client (partner.tiktokshop.com).

Auth: app key + app secret, HMAC-SHA256 signing, creator OAuth access token.
Signing scheme corroborated by independent integration write-ups
(see docs/RESEARCH/tiktok_shop_api.md "Source links"); the exact
canonicalization MUST be re-verified against Partner Center docv2 in the
sandbox before production use — a wrong guess here fails closed (40001).

Endpoint paths: the docv2 pages are login-gated, so affiliate-creator paths
are NOT hardcoded as fact. They live in ENDPOINTS as None until confirmed
in the sandbox; use scripts/probe_shop_api.py to verify candidates, then
fill them in. call() refuses to fire an unverified path with a clear error.

Human steps (not code):
  1. Register app at partner.tiktokshop.com (Affiliate developer type).
  2. Creator authorizes via Seller Center / TikTok app -> access token.
  3. App review before production (sandbox available meanwhile).

Stdlib only. Inject `http` transport for tests.
"""
from __future__ import annotations

import hashlib
import hmac
import json
import time
import urllib.parse
import urllib.request

BASE = "https://open-api.tiktokglobalshop.com"

# Logical name -> versioned path. None = NOT yet verified in docv2 sandbox.
# Fill these after running scripts/probe_shop_api.py with sandbox keys.
ENDPOINTS: dict[str, str | None] = {
    # Hunter: search Affiliate Product Marketplace (open collaboration)
    "affiliate_open_collab_search": None,
    # Hunter: product detail lookup by IDs
    "affiliate_open_collab_products_by_ids": None,
    # Content: manage creator showcase (up to 2000 products)
    "showcase_list": None,
    "showcase_add": None,
    # Content: generate affiliate promotion links
    "affiliate_link_generate": None,
    # Analyst: affiliate orders = commission reconciliation feed
    "affiliate_orders_search": None,
    # Analyst: sample application tracking
    "sample_applications_search": None,
}


def sign_request(app_secret: str, path: str,
                 query_params: dict, body: dict | None = None,
                 timestamp: int | None = None) -> tuple[str, str]:
    """Return (signature, timestamp).

    Construction: app_secret + path + sorted(key+value, excluding sign and
    access_token) + compact-JSON body + app_secret, HMAC-SHA256 keyed with
    app_secret. `timestamp` is part of the signed query params.
    """
    ts = str(timestamp if timestamp is not None else int(time.time()))
    params = {k: v for k, v in query_params.items()
              if k not in ("sign", "access_token")}
    params["timestamp"] = ts
    flat = "".join(f"{k}{params[k]}" for k in sorted(params))
    base = path + flat
    if body is not None:
        base += json.dumps(body, separators=(",", ":"))
    to_sign = app_secret + base + app_secret
    sig = hmac.new(app_secret.encode(), to_sign.encode(),
                   hashlib.sha256).hexdigest()
    return sig, ts


def _default_http(method: str, url: str, headers: dict,
                  data: bytes | None):
    req = urllib.request.Request(url, data=data, headers=headers,
                                 method=method)
    try:
        with urllib.request.urlopen(req, timeout=30) as r:
            raw = r.read()
            ctype = r.headers.get("Content-Type", "")
            return r.status, (json.loads(raw) if "json" in ctype else raw)
    except urllib.error.HTTPError as e:
        body = e.read()[:500]
        raise RuntimeError(f"HTTP {e.code} {url}: {body!r}")


class ShopClient:
    def __init__(self, app_key: str, app_secret: str, access_token: str,
                 shop_cipher: str = "", http=None):
        self.app_key = app_key
        self.app_secret = app_secret
        self.access_token = access_token
        self.shop_cipher = shop_cipher
        self.http = http or _default_http

    def call(self, name: str, params: dict | None = None,
             body: dict | None = None, method: str = "GET",
             _path_override: str | None = None) -> dict:
        """Call a registered endpoint by logical name."""
        path = _path_override or ENDPOINTS.get(name)
        if not path:
            raise RuntimeError(
                f"endpoint '{name}' has no verified path yet — "
                "confirm it in Partner Center docv2 sandbox, then fill "
                "ENDPOINTS in tiktok/shop/client.py")
        params = dict(params or {})
        params["app_key"] = self.app_key
        if self.shop_cipher:
            params["shop_cipher"] = self.shop_cipher
        sig, ts = sign_request(self.app_secret, path, params, body)
        params["timestamp"] = ts
        params["sign"] = sig
        url = BASE + path + "?" + urllib.parse.urlencode(params)
        headers = {"x-tts-access-token": self.access_token,
                   "Content-Type": "application/json"}
        data = (json.dumps(body, separators=(",", ":")).encode()
                if body is not None else None)
        status, resp = self.http(method, url, headers, data)
        code = (resp.get("code") if isinstance(resp, dict) else None)
        if code not in (0, None):
            raise RuntimeError(f"shop API error {code}: "
                               f"{str(resp)[:300]}")
        return resp.get("data", resp) if isinstance(resp, dict) else resp

    # ---------- convenience wrappers (Hunter / Content / Analyst) ----------

    def search_open_collab_products(self, keyword: str = "",
                                    category_id: str = "",
                                    min_commission_rate: int = 0,
                                    page: int = 1,
                                    page_size: int = 50) -> dict:
        """Hunter: products on the Affiliate Marketplace open to collab."""
        params = {"page_number": page, "page_size": page_size}
        if keyword:
            params["keyword"] = keyword
        if category_id:
            params["category_id"] = category_id
        if min_commission_rate:
            params["min_commission_rate"] = min_commission_rate
        return self.call("affiliate_open_collab_search", params)

    def affiliate_orders(self, start_ts: int, end_ts: int,
                         page: int = 1, page_size: int = 50) -> dict:
        """Analyst: creator affiliate orders (commission reconciliation)."""
        return self.call("affiliate_orders_search", {
            "start_time": start_ts, "end_time": end_ts,
            "page_number": page, "page_size": page_size})
