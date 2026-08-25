#!/usr/bin/env python3
"""由 tools/list JSON 產生 docs/TOOL_CATALOG.md（依資料來源分組）。

用法：gen_catalog.py <version> <tools_json>
（version 僅供頁首標註；tools_json 為 tools/list result.tools 的 JSON 字串）
輸出寫到 stdout。
"""
import json
import sys

# 分組顯示順序
GROUP_ORDER = [
    "盤中即時（TWSE-MIS）",
    "TWSE OpenAPI（TWSE-API）",
    "TWSE Web（TWSE-WEB）",
    "TWSE ETF 平台（e添富 / etfDiv）",
    "TPEx（上櫃）",
    "MOPS",
    "TAIFEX API",
    "TAIFEX 歷史下載（TAIFEX-DL）",
    "跨來源聚合",
    "篩選與衍生評分",
    "基礎設施",
]

# 盤中引擎工具（描述未必標注來源，以名稱判定）
INTRADAY_TOOLS = {
    "set_active_watchlist",
    "get_intraday_kline",
    "get_intraday_quote",
    "get_intraday_vwap",
    "detect_volume_surge",
    "scan_daytrade_eligibility",
    "get_realtime_quote",
}

# 描述無法判定時的明確覆寫表（依任務書登錄之資料源）
NAME_SOURCE = {
    "get_put_call_ratio": "TAIFEX API",
    "get_large_trader_positions": "TAIFEX API",
    "get_daily_day_trading_targets": "TWSE Web（TWSE-WEB）",
    "get_daily_securities_lending_volume": "TWSE Web（TWSE-WEB）",
    "get_first_listed_foreign_stocks_daily": "TWSE Web（TWSE-WEB）",
    "get_margin_loan_restrictions_announcement": "TWSE Web（TWSE-WEB）",
    "get_odd_lot_trading_quotes": "TWSE Web（TWSE-WEB）",
    "get_securities_trading_changes": "TWSE Web（TWSE-WEB）",
    "get_stock_price_changes": "TWSE Web（TWSE-WEB）",
    "get_stocks_no_price_change_first_five_days": "TWSE Web（TWSE-WEB）",
    "get_suspended_day_trading_announcement": "TWSE Web（TWSE-WEB）",
    "get_suspended_day_trading_history": "TWSE Web（TWSE-WEB）",
    "get_suspended_trading_stocks": "TWSE Web（TWSE-WEB）",
    "get_symbol_list": "基礎設施",
}

# 名稱前綴 → 預設來源（描述無明確標記時使用）
PREFIX_SOURCE = {
    "get_broker": "TWSE OpenAPI（TWSE-API）",
    "get_warrant": "TWSE OpenAPI（TWSE-API）",
    "get_company": "TWSE OpenAPI（TWSE-API）",
    "get_companies": "TWSE OpenAPI（TWSE-API）",
    "get_public_company": "TWSE OpenAPI（TWSE-API）",
    "get_otc_": "TPEx（上櫃）",
}


def tags_of(desc: str) -> set:
    """從描述解析出現的來源標記集合。"""
    tags = set()
    if "TWSE-API" in desc:
        tags.add("api")
    if "TWSE-WEB" in desc:
        tags.add("web")
    if "TPEx" in desc or "上櫃" in desc:
        tags.add("tpex")
    if "MOPS" in desc:
        tags.add("mops")
    if "TAIFEX-DL" in desc or ("回溯" in desc and "TAIFEX" in desc):
        tags.add("taifex_dl")
    elif "TAIFEX" in desc:
        tags.add("taifex_api")
    if "e添富" in desc or "etfDiv" in desc:
        tags.add("etf")
    if "mis.twse.com.tw" in desc or "8 秒採樣" in desc:
        tags.add("mis")
    return tags


def classify(tool) -> str:
    """回傳工具所屬分組名稱。"""
    name = tool["name"]
    desc = " ".join((tool.get("description") or "").split())

    if name in INTRADAY_TOOLS:
        return "盤中即時（TWSE-MIS）"
    if name in {"screen_stocks", "screen_high_yield",
                "get_financial_health_check"}:
        return "篩選與衍生評分"
    if name == "get_stock_trend_composite":
        return "跨來源聚合"
    if name in NAME_SOURCE:
        return NAME_SOURCE[name]

    tags = tags_of(desc)
    # ETF 工具以 e添富/etfDiv 為主源，不受描述中「上櫃」字樣影響
    if "etf" in tags:
        return "TWSE ETF 平台（e添富 / etfDiv）"
    # 單一來源直接歸組
    mapping = {
        frozenset({"mis"}): "盤中即時（TWSE-MIS）",
        frozenset({"api"}): "TWSE OpenAPI（TWSE-API）",
        frozenset({"web"}): "TWSE Web（TWSE-WEB）",
        frozenset({"etf"}): "TWSE ETF 平台（e添富 / etfDiv）",
        frozenset({"tpex"}): "TPEx（上櫃）",
        frozenset({"mops"}): "MOPS",
        frozenset({"taifex_api"}): "TAIFEX API",
        frozenset({"taifex_dl"}): "TAIFEX 歷史下載（TAIFEX-DL）",
    }
    key = frozenset(tags)
    if key in mapping:
        return mapping[key]
    # 描述未標記來源：依名稱前綴推斷
    for prefix, group in PREFIX_SOURCE.items():
        if name.startswith(prefix):
            return group
    if not tags:
        return "基礎設施"
    return "跨來源聚合"


def main() -> None:
    version = sys.argv[1]
    try:
        tools = sorted(json.loads(sys.argv[2]), key=lambda t: t["name"])
    except (json.JSONDecodeError, KeyError, IndexError) as exc:
        sys.exit(f"gen_catalog: tools/list JSON 解析失敗: {exc}")

    groups: dict = {}
    for t in tools:
        groups.setdefault(classify(t), []).append(t)

    lines = [
        f"# 附錄：完整工具目錄（{len(tools)} 個，依資料來源分組）",
        "",
        f"> 由真實服務 `tools/list` 輸出自動彙出（v{version}）；更新方式：`make catalog`。",
        "> 各工具之 Envelope、`_lineage`、快取政策與真實呼叫快照見",
        "> `snapshots/raw/<tool>.json`。",
        "",
        "## 分組統計",
        "",
        "| 資料來源 | 工具數 |",
        "| --- | --- |",
    ]
    ordered = [g for g in GROUP_ORDER if g in groups] + \
              [g for g in groups if g not in GROUP_ORDER]
    for g in ordered:
        lines.append(f"| {g} | {len(groups[g])} |")
    lines.append(f"| **合計** | **{len(tools)}** |")

    for g in ordered:
        lines.append("")
        lines.append(f"## {g}（{len(groups[g])}）")
        lines.append("")
        for t in sorted(groups[g], key=lambda x: x["name"]):
            desc = " ".join((t.get("description") or "").split())
            lines.append(f"- `{t['name']}`：{desc}")

    lines.append("")
    lines.append("## 附錄：字母排序索引")
    lines.append("")
    index = "・".join(f"`{t['name']}`" for t in tools)
    lines.append(index)

    sys.stdout.write("\n".join(lines) + "\n")


if __name__ == "__main__":
    main()
