#!/usr/bin/env python3
"""呼叫 tw-quant-mcp 全部已註冊工具（252+），保存每個工具的 JSON 結果到 snapshots/raw/。

工具清單與 inputSchema 於執行時自 tools/list 動態取得；
ARGS 僅存放人工調校的測試參數（覆寫自動產生值），
未列入者依 schema 自動產生最小合法參數。
"""

import datetime
import json
import os
import subprocess
import sys
import time

BIN = os.path.expanduser("~/Projects/tw-quant-mcp/bin/tw-quant-mcp")
RAW_DIR = os.path.expanduser("~/Projects/tw-quant-mcp/snapshots/raw")
try:
    os.makedirs(RAW_DIR, exist_ok=True)
except OSError as e:
    print(f"無法建立快照目錄 {RAW_DIR}: {e}", file=sys.stderr)
    sys.exit(1)


def recent_trading_date() -> str:
    """回傳最近一個平日的 ISO 日期字串（簡化：跳過週末）。"""
    d = datetime.date.today()
    while d.weekday() >= 5:
        d -= datetime.timedelta(days=1)
    return d.isoformat()


def _recent(days_ago: int) -> str:
    """回傳 days_ago 天前的平日 ISO 日期字串。"""
    d = datetime.date.today() - datetime.timedelta(days=max(1, days_ago))
    while d.weekday() >= 5:
        d -= datetime.timedelta(days=1)
    return d.isoformat()


# 每個工具的人工調校參數（覆蓋自動產生值；僅列需特別指定者）
ARGS = {
    # ── 盤中引擎（需先 set_active_watchlist）──
    "set_active_watchlist": {"symbols": ["2330", "2317", "2454"]},
    "get_intraday_kline": {"symbol": "2330", "timeframe": "1m", "limit": 10},
    "get_intraday_quote": {"symbol": "2330"},
    "get_intraday_vwap": {"symbol": "2330"},
    "detect_volume_surge": {"symbol": "2330", "minutes": 5},
    "scan_daytrade_eligibility": {"symbol": "2330"},
    # ── 行情/基本面 ──
    "get_stock_daily_quote": {"symbol": "2330"},
    "get_stock_daily_kline": {"symbol": "2330", "period": "day"},
    "get_institutional_investors": {"market": "tse"},
    "get_foreign_shareholding_history": {"symbol": "1101", "range": 3},
    "get_margin_trading": {"symbol": "2330"},
    "get_abnormal_trading": {"market": "otc", "top_n": 5},
    "get_warrant_activity": {"top_n": 5},
    "get_attention_disposition_stocks": {"market": "otc"},
    "get_financial_statements": {"symbol": "1232"},
    "get_monthly_revenue": {"symbol": "2330", "years": 2},
    "get_financial_health_check": {"symbol": "2330"},
    "get_valuation_ratios": {"symbol": "2330"},
    "get_esg_report": {"symbol": "2330"},
    "get_company_profile": {"symbol": "2330"},
    "screen_stocks": {"limit": 5},
    "get_dividend_history": {"symbol": "2330"},
    "screen_high_yield": {"limit": 5},
    "get_symbol_list": {"market": "tse"},
    # ── TAIFEX ──
    "get_futures_daily_ohlc": {"contract": "TX"},
    "get_futures_history": {
        "contract": "TX",
        "start": "2026-07-28",
        "end": "2026-07-31",
    },
    "get_institutional_futures_history": {"start": "2026-07-28", "end": "2026-07-31"},
    # ── 歷史查詢（schema 未標 required 但 handler 必填 start/end，T240 教訓）──
    "get_futures_daily_history": {"start": _recent(14), "end": _recent(1)},
    "get_options_daily_history": {"start": _recent(14), "end": _recent(1)},
    "get_institutional_total_history": {"start": _recent(14), "end": _recent(1)},
    "get_institutional_fut_opt_split_history": {
        "start": _recent(14),
        "end": _recent(1),
    },
    "get_institutional_traders_by_futures_history": {
        "start": _recent(14),
        "end": _recent(1),
    },
    "get_options_institutional_by_contract_history": {
        "start": _recent(14),
        "end": _recent(1),
    },
    "get_options_institutional_calls_puts_history": {
        "start": _recent(14),
        "end": _recent(1),
    },
    "get_large_traders_futures_history": {
        "contract": "TX",
        "start": _recent(14),
        "end": _recent(1),
    },
    # ── T237 上櫃治理（kind 無預設值）──
    "get_otc_governance": {"kind": "major_shareholders"},
    # ── 鉅額交易（date 必填，用最近交易日）──
    "get_block_trades_detail": {"date": recent_trading_date()},
    # ── T204 三大法人週別 ──
    "get_insti_weekly": {"type": "general"},
    # ── T205/T206 ──
    "get_final_settlement_price": {"category": "futures", "contract": "南亞"},
    "get_settled_positions": {"category": "futures", "contract": "南亞"},
    # ── T209 保證金 ──
    "get_fx_margin": {"contract": "人民幣"},
    "get_ir_margin": {},
    "get_gold_margin": {"contract": "黃金"},
    "get_etf_margin": {},
    # ── T212 興櫃 ──
    "get_emerging_quotes": {"code": "1260"},
    # ── T215/T239 財報 ──
    "get_emerging_financial_statements": {"code": "1260", "statement": "income"},
    "get_otc_financial_statements": {"code": "1240", "statement": "income"},
    # ── T224 券商營業金額 ──
    "get_otc_broker_turnover": {"level": "branch"},
    # ── T227 黃金 ──
    "get_gold_spot": {"kind": "spot", "code": "AU9901"},
    # ── T228/T229 ──
    "get_open_end_fund": {"kind": "latest", "code": "T1001Y"},
    "get_gisa_board": {"kind": "company"},
    # ── T235 期貨商交易量 ──
    "get_fcm_volume_reports": {"freq": "daily", "market": "fut"},
    # ── T236 價差 ──
    "get_calendar_spread_trades": {"kind": "summary"},
    # ── T240 信用交易細項 ──
    "get_otc_margin_sbl_detail": {"kind": "used"},
    # ── T241 制度面 ──
    "get_otc_trading_system_info": {"kind": "cmode"},
    # ── T242/T243 ──
    "get_twse_announcement_notice": {},
    "get_etf_performance": {"symbol": "0050", "limit": 5},
    "get_etf_dividend_detail": {"symbol": "0056"},
}

# 工具名 → 參數欄位名對應之範例值（自動產生 required 欄位時使用）
SYMBOL_FIELDS = {"symbol", "code", "stock_no", "etf_id", "etfid"}
DATE_FIELDS = {"date", "start", "end"}


def sample_value(tool: str, prop_name: str, spec) -> object:
    """由 schema 產生單一參數之合理範例值。"""
    if not isinstance(spec, dict):
        return "1"
    if "enum" in spec and spec["enum"]:
        return spec["enum"][0]
    t = spec.get("type")
    if isinstance(t, list):
        t = next((x for x in t if x != "null"), t[0])
    if prop_name == "symbols":
        return ["2330"]
    if prop_name in SYMBOL_FIELDS:
        return (
            "0050"
            if "etf" in tool
            else ("0050" if "etf" in str(spec.get("description", "")) else "2330")
        )
    if prop_name == "contract":
        return "TX"
    if prop_name in DATE_FIELDS:
        return recent_trading_date()
    if t in ("integer", "number"):
        v = spec.get("minimum")
        if isinstance(v, (int, float)):
            try:
                return max(2, int(v)) if prop_name in ("limit", "top_n") else int(v)
            except (TypeError, ValueError):
                return 2
        return 2
    if t == "boolean":
        return False
    if t == "array":
        return ["2330"]
    ex = spec.get("example") or spec.get("default")
    if ex is not None:
        return ex
    return "2330"


# 從 test_all_endpoints.py 匯入成熟之 build_args（偵測描述「必填」、
# 歷史工具自動帶起訖日、選填參數帶 default 等規則）。
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
try:
    from test_all_endpoints import build_args as _endpoint_build_args

    def gen_args(tool_name: str, schema: dict) -> dict:
        return _endpoint_build_args(tool_name, schema)
except ImportError:  # 後備：僅填 required 欄位

    def gen_args(tool_name: str, schema: dict) -> dict:
        args = {}
        for pname in schema.get("required", []):
            spec = schema.get("properties", {}).get(pname, {})
            args[pname] = sample_value(tool_name, pname, spec)
        return args


def send_recv(proc, obj, want_id, timeout=90):
    proc.stdin.write(json.dumps(obj, ensure_ascii=False) + "\n")
    proc.stdin.flush()
    deadline = time.time() + timeout
    while time.time() < deadline:
        line = proc.stdout.readline()
        if not line:
            break
        line = line.strip()
        if not line:
            continue
        try:
            msg = json.loads(line)
        except json.JSONDecodeError:
            continue  # 非 JSON 行（如進度輸出），忽略
        if msg.get("id") == want_id:
            return msg
    return {"jsonrpc": "2.0", "id": want_id, "error": {"message": "timeout"}}


def main():
    proc = subprocess.Popen(
        [BIN],
        stdin=subprocess.PIPE,
        stdout=subprocess.PIPE,
        stderr=subprocess.DEVNULL,
        text=True,
        bufsize=1,
    )
    if proc.stdin is None or proc.stdout is None:
        print("無法建立 stdio 管線", file=sys.stderr)
        sys.exit(1)

    # initialize 握手
    init = {
        "jsonrpc": "2.0",
        "id": 1,
        "method": "initialize",
        "params": {
            "protocolVersion": "2025-11-25",
            "capabilities": {},
            "clientInfo": {"name": "snapshot-cli", "version": "2.0.0"},
        },
    }
    send_recv(proc, init, 1, timeout=15)
    try:
        proc.stdin.write('{"jsonrpc":"2.0","method":"notifications/initialized"}\n')
        proc.stdin.flush()
    except BrokenPipeError as e:
        print(f"伺服器提前關閉: {e}", file=sys.stderr)
        sys.exit(1)

    # 取得全部工具清單與 schema
    resp = send_recv(proc, {"jsonrpc": "2.0", "id": 2, "method": "tools/list"}, 2)
    tools = resp.get("result", {}).get("tools", [])
    print(f"tools/list 回傳 {len(tools)} 個工具", flush=True)

    # 盤中引擎工具需先設定 watchlist
    if any(
        t["name"].startswith("get_intraday") or t["name"] == "detect_volume_surge"
        for t in tools
    ):
        wl = ARGS.get("set_active_watchlist", {"symbols": ["2330"]})
        print(f"前置：set_active_watchlist {wl['symbols']}", flush=True)
        r = send_recv(
            proc,
            {
                "jsonrpc": "2.0",
                "id": 5,
                "method": "tools/call",
                "params": {"name": "set_active_watchlist", "arguments": wl},
            },
            5,
        )
        try:
            with open(os.path.join(RAW_DIR, "set_active_watchlist.json"), "w") as f:
                json.dump(
                    {"arguments": wl, "response": r}, f, ensure_ascii=False, indent=2
                )
        except OSError as e:
            print(f"寫檔失敗: {e}", file=sys.stderr)

    ok, err = 0, 0
    failed = []
    msg_id = 10
    for i, t in enumerate(tools):
        name = t["name"]
        if name == "set_active_watchlist":
            continue  # 已於前置步驟呼叫
        schema = t.get("inputSchema", {})
        args = dict(gen_args(name, schema))
        args.update(ARGS.get(name, {}))  # 人工調校覆寫
        print(f"[{i + 1}/{len(tools)}] 呼叫 {name} ...", flush=True)
        resp = send_recv(
            proc,
            {
                "jsonrpc": "2.0",
                "id": msg_id,
                "method": "tools/call",
                "params": {"name": name, "arguments": args},
            },
            msg_id,
        )
        is_err = resp.get("result", {}).get("isError", False)
        has_err = "error" in resp
        if is_err or has_err:
            err += 1
            failed.append(name)
        else:
            ok += 1
        try:
            with open(os.path.join(RAW_DIR, f"{name}.json"), "w") as f:
                json.dump(
                    {"arguments": args, "response": resp},
                    f,
                    ensure_ascii=False,
                    indent=2,
                )
        except OSError as e:
            print(f"寫檔失敗: {e}", file=sys.stderr)
        msg_id += 1
        time.sleep(0.5)

    proc.terminate()
    print(f"\n完成：{ok} 成功 / {err} 失敗，共 {len(tools)} 個結果已存至 {RAW_DIR}")
    if failed:
        print("失敗清單：")
        for n in failed:
            print(f"  - {n}")
    return 1 if err else 0


if __name__ == "__main__":
    sys.exit(main())
