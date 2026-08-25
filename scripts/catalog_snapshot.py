#!/usr/bin/env python3
"""三個官方 OpenAPI 目錄（TWSE/TPEx/TAIFEX）端點快照與差異比對。

用法：
  catalog_snapshot.py check            # 與 baseline 比對，輸出新增/刪減（有變更 exit 1）
  catalog_snapshot.py update           # 抓取現況並寫入新 baseline

baseline 存於 snapshots/catalogs/<source>_endpoints.txt（每行一條路徑）。
"""

import json
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
BASELINE = ROOT / "snapshots" / "catalogs"

SOURCES = {
    "twse": {
        "url": "https://openapi.twse.com.tw/v1/swagger.json",
    },
    "tpex": {
        "url": "https://www.tpex.org.tw/openapi/swagger.json",
    },
    "taifex": {
        "url": "https://openapi.taifex.com.tw/swagger.json",
    },
}


def fetch_paths(name: str) -> list:
    """以 curl（清單參數、無 shell）抓取 swagger 並回傳排序後的路徑清單。"""
    import subprocess
    import urllib.parse

    url = SOURCES[name]["url"]
    if urllib.parse.urlparse(url).scheme != "https":
        raise ValueError(f"僅允許 https URL，實際：{url}")
    try:
        res = subprocess.run(
            ["curl", "-sSL", "--max-time", "60", url],
            capture_output=True,
            text=True,
            check=True,
        )
        doc = json.loads(res.stdout)
    except (subprocess.CalledProcessError, json.JSONDecodeError) as exc:
        raise RuntimeError(f"{name} 目錄抓取/解析失敗：{exc}") from exc
    return sorted(p.strip("/").lstrip("v1/") for p in doc.get("paths", {}))


def load_baseline(name: str) -> set:
    f = BASELINE / f"{name}_endpoints.txt"
    if not f.exists():
        return set()
    return {line.strip() for line in f.read_text().splitlines() if line.strip()}


def main() -> None:
    mode = sys.argv[1] if len(sys.argv) > 1 else "check"
    if mode not in ("check", "update"):
        sys.exit(f"用法：{sys.argv[0]} check|update")

    BASELINE.mkdir(parents=True, exist_ok=True)
    changed = False

    for name in SOURCES:
        print(f"==> 抓取 {name} 目錄…")
        try:
            current = set(fetch_paths(name))
        except Exception as exc:
            print(f"    ✗ {name} 抓取失敗：{exc}")
            if mode == "check":
                changed = True  # 抓取失敗視為需關注
            continue

        basefile = BASELINE / f"{name}_endpoints.txt"
        baseline = load_baseline(name)

        if mode == "update":
            basefile.write_text("\n".join(sorted(current)) + "\n")
            print(f"    ✓ baseline 更新：{len(current)} 條")
            continue

        added = sorted(current - baseline)
        removed = sorted(baseline - current)
        if not baseline:
            print(f"    ! 尚無 baseline（{name}），請先執行 update")
            changed = True
            continue
        if not added and not removed:
            print(f"    ✓ 無變更（{len(current)} 條）")
            continue

        changed = True
        print(f"    ⚠ 偵測到變更（+{len(added)} / -{len(removed)}）")
        for p in added:
            print(f"      + {p}")
        for p in removed:
            print(f"      - {p}")

    if mode == "check":
        if changed:
            print(
                "\n結果：官方目錄有變更或異常——請評估新增工具/移除標註，"
                "確認後執行 make catalog-snapshot 更新 baseline。"
            )
            sys.exit(1)
        print("\n結果：三個官方目錄皆無變更。")


if __name__ == "__main__":
    main()
