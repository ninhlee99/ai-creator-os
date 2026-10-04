#!/usr/bin/env python3
"""
Lấy YouTube refresh token cho AI Creator OS (chạy 1 lần trên máy Mac của Ninh).

Cần trước: Google Cloud project đã bật "YouTube Data API v3",
OAuth client loại "Desktop app" (lấy Client ID + Client Secret).

Cách dùng:
  python3 scripts/get-youtube-token.py --client-id '...' \
      --client-secret '...' --username kechuyen_ai_yt \
      --data-dir "$HOME/aicos-data"

Script mở trình duyệt → Ninh đăng nhập Google, chọn kênh YouTube,
bấm Cho phép → token được ghi vào <data-dir>/tokens/youtube_token_<username>.json
dưới dạng {"refresh_token": "..."} — đúng định dạng app đọc.
"""
import argparse, http.server, json, os, socket, sys, urllib.parse, urllib.request, webbrowser

AUTH_URL = "https://accounts.google.com/o/oauth2/v2/auth"
TOKEN_URL = "https://oauth2.googleapis.com/token"
SCOPE = ("https://www.googleapis.com/auth/youtube.upload"
         " https://www.googleapis.com/auth/yt-analytics.readonly")
# Đợt H3: thêm yt-analytics.readonly để growth lấy view 30 ngày/watch hours.
# Token cũ (chỉ youtube.upload) vẫn đăng video được, nhưng Analytics sẽ 403 —
# chạy lại script này để cấp thêm scope.


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--client-id", required=True)
    ap.add_argument("--client-secret", required=True)
    ap.add_argument("--username", required=True, help="username tài khoản trong app")
    ap.add_argument("--data-dir", required=True, help="thư mục data của app")
    a = ap.parse_args()

    code_holder = {}

    class H(http.server.BaseHTTPRequestHandler):
        def do_GET(self):
            q = urllib.parse.parse_qs(urllib.parse.urlparse(self.path).query)
            if "code" in q:
                code_holder["code"] = q["code"][0]
                self.send_response(200)
                self.send_header("Content-Type", "text/html; charset=utf-8")
                self.end_headers()
                self.wfile.write(
                    "<h1>Xong! Quay lại terminal.</h1>".encode("utf-8"))
            else:
                self.send_response(400); self.end_headers()
        def log_message(self, *x):
            pass

    srv = http.server.HTTPServer(("127.0.0.1", 0), H)
    port = srv.server_address[1]
    redirect = f"http://127.0.0.1:{port}/"
    params = {
        "client_id": a.client_id, "redirect_uri": redirect,
        "response_type": "code", "scope": SCOPE,
        "access_type": "offline", "prompt": "consent",
    }
    url = AUTH_URL + "?" + urllib.parse.urlencode(params)
    print("Mở trình duyệt để cấp quyền...")
    webbrowser.open(url)
    print("Nếu trình duyệt không tự mở, dán link này:\n" + url + "\n")
    print("Chờ xác thực (tối đa 3 phút)...")
    srv.timeout = 180
    srv.handle_request()
    srv.server_close()
    if "code" not in code_holder:
        sys.exit("Hết giờ chờ hoặc bị từ chối. Chạy lại.")

    data = urllib.parse.urlencode({
        "code": code_holder["code"], "client_id": a.client_id,
        "client_secret": a.client_secret, "redirect_uri": redirect,
        "grant_type": "authorization_code",
    }).encode()
    req = urllib.request.Request(TOKEN_URL, data=data)
    with urllib.request.urlopen(req, timeout=30) as r:
        tok = json.load(r)
    refresh = tok.get("refresh_token")
    if not refresh:
        sys.exit(f"Google không trả refresh_token: {tok}")

    safe = "".join(c if c.isalnum() or c in "-_" else "_" for c in a.username)
    tdir = os.path.join(a.data_dir, "tokens")
    os.makedirs(tdir, exist_ok=True)
    out = os.path.join(tdir, f"youtube_token_{safe}.json")
    with open(out, "w") as f:
        json.dump({"refresh_token": refresh}, f)
    os.chmod(out, 0o600)
    print(f"Đã ghi {out} (chỉ chứa refresh_token, quyền 600).")
    print("Mở app → Tài khoản → tab Kết nối: YouTube sẽ hiện 'đã cấu hình'.")


if __name__ == "__main__":
    main()
