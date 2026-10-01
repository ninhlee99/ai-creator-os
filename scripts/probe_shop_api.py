"""Probe TikTok Shop API endpoint paths in the sandbox.

The docv2 pages are login-gated, so affiliate-creator paths must be
confirmed empirically. Usage (with sandbox app credentials):

    TIKTOK_SHOP_APP_KEY=... TIKTOK_SHOP_APP_SECRET=... \\
    TIKTOK_SHOP_ACCESS_TOKEN=... TIKTOK_SHOP_CIPHER=... \\
    python3 scripts/probe_shop_api.py

For each candidate path it prints the HTTP/API status so you can copy the
working path into ENDPOINTS in tiktok/shop/client.py.
"""
import os
import sys

sys.path.insert(0, ".")

from tiktok.shop.client import ShopClient  # noqa: E402

# Candidate paths collected from API mirrors of the official docs.
# Each will be tried; the sandbox tells us which are real for this region.
CANDIDATES = {
    "affiliate_open_collab_search": [
        "/affiliate_creator/202405/products/search",
        "/affiliate_creator/202501/products/search",
        "/affiliate/202405/products/search",
    ],
    "affiliate_orders_search": [
        "/affiliate_creator/202405/orders/search",
        "/affiliate_creator/202501/orders/search",
    ],
    "affiliate_link_generate": [
        "/affiliate_creator/202405/promotion_links/generate",
    ],
}


def main() -> None:
    client = ShopClient(
        os.environ["TIKTOK_SHOP_APP_KEY"],
        os.environ["TIKTOK_SHOP_APP_SECRET"],
        os.environ["TIKTOK_SHOP_ACCESS_TOKEN"],
        os.environ.get("TIKTOK_SHOP_CIPHER", ""))
    for name, paths in CANDIDATES.items():
        for path in paths:
            try:
                client.call(name, {"page_number": 1, "page_size": 1},
                            _path_override=path)
                print(f"OK    {name:35s} {path}")
            except Exception as e:  # noqa: BLE001
                print(f"FAIL  {name:35s} {path}  -> {str(e)[:90]}")


if __name__ == "__main__":
    main()
