"""Tests for TikTok API clients (fake transports, no network)."""
import hashlib
import hmac
import json
import os
import sys
import tempfile

sys.path.insert(0, ".")

from tiktok.posting.client import PostingClient, TokenStore
from tiktok.shop.client import ENDPOINTS, ShopClient, sign_request


# ---------- shop signing ----------

def test_sign_deterministic_and_excludes_secrets():
    p = {"app_key": "K", "keyword": "sach"}
    s1, ts1 = sign_request("SECRET", "/x/y", p, timestamp=1700000000)
    s2, ts2 = sign_request("SECRET", "/x/y", p, timestamp=1700000000)
    assert s1 == s2 and ts1 == ts2 == "1700000000"
    # manual recomputation per documented construction
    flat = "app_keyK" + "keyword" + "sach" + "timestamp1700000000"
    expect = hmac.new(b"SECRET",
                      ("SECRET" + "/x/y" + flat + "SECRET").encode(),
                      hashlib.sha256).hexdigest()
    assert s1 == expect
    # sign/access_token must not affect the signature
    s3, _ = sign_request("SECRET", "/x/y",
                         {**p, "sign": "old", "access_token": "tok"},
                         timestamp=1700000000)
    assert s3 == s1


def test_sign_includes_body_bytes():
    s1, _ = sign_request("S", "/p", {}, body={"a": 1}, timestamp=1)
    s2, _ = sign_request("S", "/p", {}, body={"a": 2}, timestamp=1)
    assert s1 != s2


def test_shop_call_refuses_unverified_endpoint():
    c = ShopClient("k", "s", "tok", http=lambda *a: (200, {}))
    try:
        c.search_open_collab_products(keyword="sach")
        raise AssertionError("should refuse")
    except RuntimeError as e:
        assert "no verified path" in str(e)


def test_shop_call_signs_and_sends():
    calls = []

    def fake_http(method, url, headers, data):
        calls.append((method, url, headers, data))
        return 200, {"code": 0, "data": {"products": []}}

    c = ShopClient("k", "s", "tok", http=fake_http)
    c.call("probe", {"page_number": 1},
           _path_override="/affiliate_creator/202405/products/search")
    method, url, headers, data = calls[0]
    assert method == "GET"
    assert "app_key=k" in url and "sign=" in url and "timestamp=" in url
    assert headers["x-tts-access-token"] == "tok"


def test_shop_api_error_raises():
    def fake_http(method, url, headers, data):
        return 200, {"code": 40001, "message": "invalid signature"}
    c = ShopClient("k", "s", "tok", http=fake_http)
    try:
        c.call("probe", _path_override="/x")
        raise AssertionError("should raise")
    except RuntimeError as e:
        assert "40001" in str(e)


# ---------- posting client ----------

def _posting_client(tmp, script):
    calls = []
    state = {"n": 0}

    def fake_http(method, url, headers, data):
        calls.append((method, url, headers, data))
        kind, payload = script(state, method, url, headers, data)
        return kind, payload

    store = TokenStore(os.path.join(tmp, "tok.json"))
    return PostingClient("CK", "CS", store, http=fake_http), calls, store


def test_posting_publish_flow():
    tmp = tempfile.mkdtemp()
    clip = os.path.join(tmp, "clip.mp4")
    with open(clip, "wb") as f:
        f.write(b"\x00" * 1024)  # 1 KB -> single chunk

    def script(state, method, url, headers, data):
        if url.endswith("/oauth/token/"):
            return 200, {"access_token": "AT", "refresh_token": "RT",
                         "expires_in": 86400}
        if url.endswith("/video/init/"):
            body = json.loads(data)
            assert body["post_info"]["privacy_level"] == "SELF_ONLY"
            assert body["source_info"]["total_chunk_count"] == 1
            return 200, {"error": {"code": "ok"},
                         "data": {"publish_id": "pid1",
                                  "upload_url": "https://up/x"}}
        if url.startswith("https://up/"):
            assert headers["Content-Range"] == "bytes 0-1023/1024"
            return 201, {}
        if url.endswith("/status/fetch/"):
            return 200, {"data": {"status": "PUBLISH_COMPLETE"}}
        raise AssertionError(url)

    client, calls, store = _posting_client(tmp, script)
    # seed a fresh token so no refresh happens
    store.save({"access_token": "AT", "refresh_token": "RT",
                "expires_in": 86400})
    pid = client.publish_file(clip, "test caption #sach")
    assert pid == "pid1"
    assert client.poll_status(pid, timeout_s=5, interval_s=0) == \
        "PUBLISH_COMPLETE"
    assert any("/video/init/" in c[1] for c in calls)


def test_posting_refreshes_expired_token():
    tmp = tempfile.mkdtemp()

    def script(state, method, url, headers, data):
        assert url.endswith("/oauth/token/")
        body = dict(p.split("=") for p in data.decode().split("&"))
        assert body["grant_type"] == "refresh_token"
        assert body["refresh_token"] == "OLD_RT"
        return 200, {"access_token": "NEW_AT", "refresh_token": "NEW_RT",
                     "expires_in": 86400}

    client, calls, store = _posting_client(tmp, script)
    # seed an EXPIRED token by writing the file directly
    # (save() would stamp obtained_at=now)
    import json as _json
    with open(store.path, "w") as f:
        _json.dump({"access_token": "OLD_AT", "refresh_token": "OLD_RT",
                    "expires_in": 86400, "obtained_at": 1}, f)
    assert client.access_token() == "NEW_AT"
    saved = store.load()
    assert saved["refresh_token"] == "NEW_RT"  # rotated token persisted
