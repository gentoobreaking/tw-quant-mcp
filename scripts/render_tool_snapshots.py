#!/usr/bin/env python3
"""將 snapshots/raw/*.json 渲染為 HTML 並用 Chrome headless 截圖為 PNG。
輸出：~/Projects/tw-quant-mcp/snapshots/<tool_name>.png
"""
import json
import os
import subprocess
import sys
import time

RAW_DIR = os.path.expanduser("~/Projects/tw-quant-mcp/snapshots/raw")
OUT_DIR = os.path.expanduser("~/Projects/tw-quant-mcp/snapshots")
CHROME = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"

ARGS = {
    "set_active_watchlist": {"symbols": ["2330", "2317", "2454"]},
    "get_intraday_kline": {"symbol": "2330", "timeframe": "1m", "limit": 10},
    "get_intraday_quote": {"symbol": "2330"},
    "get_intraday_vwap": {"symbol": "2330"},
    "detect_volume_surge": {"symbol": "2330", "minutes": 5},
    "scan_daytrade_eligibility": {"symbol": "2330"},
    "get_stock_daily_quote": {"symbol": "2330"},
    "get_stock_daily_kline": {"symbol": "2330", "period": "day"},
    "get_market_summary": {},
    "get_institutional_investors": {"market": "tse"},
    "get_foreign_industry_holdings": {},
    "get_foreign_shareholding_history": {"symbol": "1101", "range": 3},
    "get_margin_trading": {"symbol": "2330"},
    "get_abnormal_trading": {"market": "otc", "top_n": 5},
    "get_warrant_activity": {"top_n": 5},
    "get_major_announcements": {},
    "get_attention_disposition_stocks": {"market": "otc"},
    "get_financial_statements": {"symbol": "1232"},
    "get_monthly_revenue": {"symbol": "2330", "years": 2},
    "get_financial_health_check": {"symbol": "2330"},
    "get_valuation_ratios": {"symbol": "2330"},
    "get_esg_report": {"symbol": "2330"},
    "get_company_profile": {"symbol": "2330"},
    "screen_stocks": {"limit": 5},
    "get_dividend_history": {"symbol": "2330"},
    "get_exdividend_calendar": {},
    "screen_high_yield": {"limit": 5},
    "get_futures_daily_ohlc": {"contract": "TX"},
    "get_futures_history": {"contract": "TX", "start": "2026-07-28", "end": "2026-07-31"},
    "get_put_call_ratio": {},
    "get_large_trader_positions": {},
    "get_institutional_futures_positions": {},
    "get_institutional_options_positions": {},
    "get_institutional_futures_history": {"start": "2026-07-28", "end": "2026-07-31"},
    "get_symbol_list": {"market": "tse"},
    "get_trading_calendar": {},
}


def summarize_data(data, depth=0, max_items=8, max_str=80):
    """遞迴摘要 data，避免截圖過長。"""
    if depth > 2:
        return "…"
    if isinstance(data, dict):
        items = []
        for k, v in list(data.items())[:max_items]:
            items.append(f"<b>{k}</b>: {summarize_data(v, depth+1)}")
        if len(data) > max_items:
            items.append(f"<i>… 共 {len(data)} 鍵</i>")
        return "{ " + "; ".join(items) + " }"
    if isinstance(data, list):
        if len(data) == 0:
            return "[]"
        inner = summarize_data(data[0], depth+1)
        return f"[{len(data)} 筆, 例: {inner}]"
    if isinstance(data, str):
        s = data if len(data) <= max_str else data[:max_str] + "…"
        return f"'{s}'"
    return str(data)


def build_html(name, result_obj):
    is_err = result_obj.get("isError", False)
    content_list = result_obj.get("content") or []
    text = content_list[0].get("text", "") if content_list else ""
    args = json.dumps(ARGS.get(name, {}), ensure_ascii=False)
    try:
        parsed = json.loads(text)
    except Exception:
        parsed = None
    status = "成功 ✅" if not is_err else "錯誤 ❌"
    color = "#0a7d32" if not is_err else "#c62828"
    badge = "#e6f4ea" if not is_err else "#fdecea"

    if parsed is not None and not is_err:
        data = parsed.get("data")
        lineage = parsed.get("_lineage", {})
        chart = parsed.get("_chart_meta")
        http_calls = parsed.get("http_calls", "?")
        data_html = summarize_data(data)
        lineage_html = "<br>".join(
            f"<b>{k}</b>: {v}" for k, v in lineage.items()
        ) if isinstance(lineage, dict) else str(lineage)
        chart_html = ""
        if chart:
            chart_html = f"<div class='kv'><b>_chart_meta</b>: {json.dumps(chart, ensure_ascii=False)[:200]}…</div>"
        body = f"""
        <div class="card">
          <div class="kv"><b>data</b>: {data_html}</div>
          <div class="kv"><b>http_calls</b>: {http_calls}</div>
          <div class="lineage"><b>_lineage</b>:<br>{lineage_html}</div>
          {chart_html}
        </div>"""
    else:
        body = f"""
        <div class="card err">
          <div class="kv"><b>訊息</b>: {text[:600]}</div>
        </div>"""

    return f"""<!DOCTYPE html>
<html><head><meta charset="utf-8">
<style>
  body {{ font-family: -apple-system, "PingFang TC", "Microsoft JhengHei", sans-serif;
         background: #f5f6fa; margin: 0; padding: 24px; color: #1a1a2e; }}
  .header {{ display: flex; align-items: center; gap: 12px; margin-bottom: 16px; }}
  .title {{ font-size: 22px; font-weight: 700; }}
  .badge {{ background: {badge}; color: {color}; padding: 4px 12px; border-radius: 12px;
            font-size: 13px; font-weight: 600; }}
  .args {{ font-family: ui-monospace, Menlo, monospace; font-size: 12px; color: #555;
           background: #eef0f5; padding: 4px 8px; border-radius: 6px; margin-bottom: 12px; }}
  .card {{ background: #fff; border: 1px solid #e0e3eb; border-radius: 10px;
           padding: 14px 16px; margin-bottom: 12px; box-shadow: 0 1px 3px rgba(0,0,0,.05); }}
  .card.err {{ border-left: 4px solid #c62828; }}
  .kv {{ font-size: 13px; line-height: 1.65; margin-bottom: 6px; overflow-wrap: break-word; }}
  .lineage {{ font-size: 12px; color: #444; background: #f8f9fc; padding: 8px 10px;
              border-radius: 6px; line-height: 1.7; }}
  .raw {{ font-family: ui-monospace, Menlo, monospace; font-size: 11px; color: #666;
          background: #fbfbfd; border: 1px dashed #ddd; padding: 10px; border-radius: 6px;
          white-space: pre-wrap; max-height: 260px; overflow: auto; }}
  h2 {{ margin: 18px 0 6px; font-size: 15px; color: #333; }}
</style></head>
<body>
  <div class="header">
    <div class="title">{name}</div>
    <div class="badge">{status}</div>
  </div>
  <div class="args">參數: {args}</div>
  {body}
  <h2>原始回應（JSON）</h2>
  <div class="raw">{json.dumps(result_obj, ensure_ascii=False, indent=2)[:4000]}</div>
</body></html>"""


def render_one(name, html_path, png_path):
    html_url = "file://" + html_path
    subprocess.run([
        CHROME, "--headless=new", "--disable-gpu", "--hide-scrollbars",
        "--force-device-scale-factor=2", "--window-size=980,760",
        f"--screenshot={png_path}", html_url,
    ], capture_output=True, timeout=60)
    return os.path.exists(png_path) and os.path.getsize(png_path) > 1000


def main():
    os.makedirs(OUT_DIR, exist_ok=True)
    tmp = os.path.join(OUT_DIR, ".html_tmp")
    os.makedirs(tmp, exist_ok=True)
    files = sorted(f for f in os.listdir(RAW_DIR) if f.endswith(".json"))
    ok, fail = 0, []
    for f in files:
        name = f[:-5]
        with open(os.path.join(RAW_DIR, f), encoding="utf-8") as fh:
            obj = json.load(fh)
        # 相容兩種儲存格式：
        #   批次腳本: {"arguments":..., "response": {"result": {...}}}
        #   one_tool: {"jsonrpc":..., "id":..., "result": {...}}
        result = obj.get("response", {}).get("result") if isinstance(obj.get("response"), dict) else None
        if result is None and isinstance(obj.get("result"), dict):
            result = obj["result"]
        if result is None:
            fail.append(name + " (無法解析 result)")
            continue
        content = result.get("content") or []
        result_obj = {
            "isError": result.get("isError", False),
            "content": content,
        }
        html = build_html(name, result_obj)
        html_path = os.path.join(tmp, name + ".html")
        with open(html_path, "w", encoding="utf-8") as fh:
            fh.write(html)
        png_path = os.path.join(OUT_DIR, name + ".png")
        if render_one(name, html_path, png_path):
            ok += 1
        else:
            fail.append(name)
        time.sleep(0.15)
    print(f"渲染完成: {ok} 成功, {len(fail)} 失敗: {fail}")
    print(f"PNG 已輸出至 {OUT_DIR}")


if __name__ == "__main__":
    main()
