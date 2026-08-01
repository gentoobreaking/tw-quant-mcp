#!/usr/bin/env python3
"""呼叫 tw-quant-mcp 全部 36 個工具，保存每個工具的 JSON 結果到 snapshots/raw/。"""
import json
import os
import subprocess
import sys
import time

BIN = os.path.expanduser("~/Projects/tw-quant-mcp/bin/tw-quant-mcp")
RAW_DIR = os.path.expanduser("~/Projects/tw-quant-mcp/snapshots/raw")
os.makedirs(RAW_DIR, exist_ok=True)

# 每個工具的測試參數（依 schema 合理填寫）
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


def call_tool(proc, tool_name, args, msg_id):
    req = {
        "jsonrpc": "2.0",
        "id": msg_id,
        "method": "tools/call",
        "params": {"name": tool_name, "arguments": args},
    }
    proc.stdin.write(json.dumps(req, ensure_ascii=False) + "\n")
    proc.stdin.flush()
    deadline = time.time() + 60
    while time.time() < deadline:
        line = proc.stdout.readline()
        if not line:
            break
        line = line.strip()
        if not line:
            continue
        try:
            obj = json.loads(line)
        except Exception:
            continue
        if obj.get("id") == msg_id:
            return obj
    return {"jsonrpc": "2.0", "id": msg_id, "error": {"message": "timeout"}}


def main():
    proc = subprocess.Popen(
        [BIN],
        stdin=subprocess.PIPE,
        stdout=subprocess.PIPE,
        stderr=subprocess.DEVNULL,
        text=True,
        bufsize=1,
    )
    # initialize
    init = {
        "jsonrpc": "2.0",
        "id": 1,
        "method": "initialize",
        "params": {
            "protocolVersion": "2025-11-25",
            "capabilities": {},
            "clientInfo": {"name": "snapshot-cli", "version": "1.0.0"},
        },
    }
    proc.stdin.write(json.dumps(init) + "\n")
    proc.stdin.flush()
    # 讀 initialize 回應
    deadline = time.time() + 15
    while time.time() < deadline:
        line = proc.stdout.readline()
        if line and json.loads(line).get("id") == 1:
            break
    proc.stdin.write('{"jsonrpc":"2.0","method":"notifications/initialized"}\n')
    proc.stdin.flush()

    results = {}
    msg_id = 10
    for name, args in ARGS.items():
        print(f"呼叫 {name} ...", flush=True)
        resp = call_tool(proc, name, args, msg_id)
        results[name] = {"arguments": args, "response": resp}
        with open(os.path.join(RAW_DIR, f"{name}.json"), "w") as f:
            json.dump(results[name], f, ensure_ascii=False, indent=2)
        msg_id += 1
        time.sleep(1.0)  # 避免太密集（MIS 8s 限制只影響盤中）

    proc.terminate()
    print(f"\n完成：{len(results)} 個工具結果已存至 {RAW_DIR}")


if __name__ == "__main__":
    main()
