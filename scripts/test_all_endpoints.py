#!/usr/bin/env python3
"""批次呼叫 tw-quant-mcp 所有已實作 MCP tools，記錄呼叫方式、回應內容與回應時間。

產出：
  - docs/ENDPOINT_TEST_REPORT.md      （彙總表格）
  - docs/endpoint_responses/<tool>.json（完整原始回應）
"""
import json
import os
import re
import sys
import time
import urllib.request
from datetime import date, timedelta

BASE_URL = os.environ.get("MCP_BASE_URL", "http://localhost:8000/")
DOCS_DIR = os.path.expanduser("~/Projects/tw-quant-mcp/docs")
RAW_DIR = os.path.join(DOCS_DIR, "endpoint_responses")
EXCERPT_LEN = 400

TODAY = date.today()


def rpc(method, params):
    body = json.dumps({"jsonrpc": "2.0", "id": 1, "method": method, "params": params}).encode()
    req = urllib.request.Request(
        BASE_URL, data=body,
        headers={"Content-Type": "application/json",
                 "Accept": "application/json, text/event-stream"},
    )
    t0 = time.perf_counter()
    try:
        with urllib.request.urlopen(req, timeout=120) as resp:
            payload = json.loads(resp.read().decode())
            elapsed_ms = (time.perf_counter() - t0) * 1000
            return resp.status, payload, elapsed_ms
    except urllib.error.HTTPError as e:
        elapsed_ms = (time.perf_counter() - t0) * 1000
        return e.code, {"error": e.read().decode()[:2000]}, elapsed_ms


def list_tools():
    _, payload, _ = rpc("tools/list", {})
    return [(t["name"], t.get("inputSchema", {})) for t in payload["result"]["tools"]]


def build_args(name, schema):
    """依 required 參數名稱填入合理預設值。"""
    props = schema.get("properties", {})
    required = schema.get("required", []) or []
    # 已知 schema 與 handler 不一致：warrant_basic_info 的 code
    # schema 標記選填，但 handler 實際必填（詳見報告備註）。
    if name == "get_warrant_basic_info":
        args0 = {"code": "2330"}
        return args0
    args = {}
    for p in required:
        if p == "date":
            args[p] = TODAY.isoformat()
        elif p in ("symbol", "code", "stock_no"):
            args[p] = "0056" if "etf" in name else "2330"
        elif p == "symbols":
            args[p] = ["2330"]
        elif p == "market":
            args[p] = "tse"
        elif p in ("start", "start_date"):
            args[p] = (TODAY - timedelta(days=7)).isoformat()
        elif p in ("end", "end_date"):
            args[p] = TODAY.isoformat()
        elif p in props and "default" in props[p]:
            args[p] = props[p]["default"]
        elif p in props and "enum" in props[p]:
            args[p] = props[p]["enum"][0]
        elif p == "contract":
            args[p] = "TX"
        elif p in ("limit",):
            args[p] = 3
        else:
            t = props.get(p, {}).get("type")
            args[p] = 5 if t == "integer" else ("2330" if t == "string" else {})
    # handler 實際必填（description 註明「必填」）但 schema 未標記者補上
    for p, spec in props.items():
        if p not in args and isinstance(spec.get("description"), str) \
                and "必填" in spec["description"]:
            args[p] = "TX" if p == "contract" else (
                (TODAY - timedelta(days=7)).isoformat() if p.startswith("start") else
                TODAY.isoformat() if p.startswith("end") else "2330")
    # 歷史區間工具即使選填也帶上起訖日，避免資料量過大被拒
    if any(k in props for k in ("start", "start_date")):
        args.setdefault("start", (TODAY - timedelta(days=7)).isoformat())
        args.setdefault("start_date", (TODAY - timedelta(days=7)).isoformat())
    if any(k in props for k in ("end", "end_date")):
        args.setdefault("end", TODAY.isoformat())
        args.setdefault("end_date", TODAY.isoformat())
    # 有 enum/default 的選填參數也帶上 default，讓回應更貼近實際使用
    for p, spec in props.items():
        if p not in args and "default" in spec and p not in ("limit", "offset", "page_number", "page_size"):
            args[p] = spec["default"]
    return args


def fmt_args(args):
    if not args:
        return "（無參數）"
    parts = []
    for k, v in args.items():
        vs = json.dumps(v, ensure_ascii=False)
        if len(vs) > 60:
            vs = vs[:57] + "..."
        parts.append(f"{k}={vs}")
    return ", ".join(parts)


def first_text(result):
    """擷取 tools/call result 中所有文字內容。"""
    content = result.get("content") or []
    texts = [c.get("text", "") for c in content if c.get("type") == "text"]
    joined = "\n".join(texts)
    return joined


def summarize(text):
    if len(text) <= EXCERPT_LEN:
        return text
    return text[:EXCERPT_LEN] + f"\n…（截斷，全文 {len(text)} 字元）"


def md_escape(s):
    return s.replace("|", "\\|").replace("\n", "<br>")


def main():
    os.makedirs(RAW_DIR, exist_ok=True)
    tools = list_tools()
    print(f"共 {len(tools)} 個 tools，開始逐一呼叫…", file=sys.stderr)

    rows = []
    ok_count = err_count = 0
    for i, (name, schema) in enumerate(tools, 1):
        args = build_args(name, schema)
        status, payload, ms = rpc("tools/call", {"name": name, "arguments": args})
        result = payload.get("result") or {}
        is_error = bool(result.get("isError")) or ("error" in payload)
        text = first_text(result)
        if not text:
            if "error" in payload:
                err = payload["error"]
                text = json.dumps(err, ensure_ascii=False)[:800] if isinstance(err, dict) else str(err)[:800]
            elif result.get("structuredContent") is not None:
                text = json.dumps(result["structuredContent"], ensure_ascii=False)
            else:
                text = "(空回應)"
        status_str = "OK" if not is_error else "ERROR"
        if is_error:
            err_count += 1
        else:
            ok_count += 1
        # 完整原始回應存檔
        with open(os.path.join(RAW_DIR, f"{name}.json"), "w") as f:
            json.dump({"request": {"name": name, "arguments": args},
                       "http_status": status,
                       "elapsed_ms": round(ms, 1),
                       "response": payload}, f, ensure_ascii=False, indent=2)
        rows.append({
            "name": name, "args": fmt_args(args), "status": status,
            "ok": not is_error, "ms": ms, "excerpt": text,
        })
        print(f"[{i}/{len(tools)}] {status_str} {name} {ms:.0f}ms", file=sys.stderr)

    # 產生報表
    lines = [
        "# tw-quant-mcp Endpoint 全量實測報告",
        "",
        f"- 測試日期：{TODAY.isoformat()}",
        f"- 測試方式：docker-compose 啟動最新版 image（container：`tw-quant-mcp`），以 MCP Streamable HTTP（JSON-RPC 2.0）對 `http://localhost:8000/` 逐一呼叫 `tools/call`。",
        f"- Tools 總數：{len(tools)}；成功：{ok_count}；錯誤：{err_count}",
        "- 回應摘錄長度上限 400 字元；**完整原始回應**存於 [`docs/endpoint_responses/<tool>.json`](endpoint_responses/)。",
        "",
        "| # | Tool | 呼叫參數 | HTTP | 結果 | 耗時(ms) | 回應摘錄 |",
        "|---|------|----------|------|------|---------|----------|",
    ]
    for i, r in enumerate(rows, 1):
        lines.append(
            f"| {i} | `{r['name']}` | {md_escape(r['args'])} | {r['status']} "
            f"| {'✅' if r['ok'] else '❌'} | {r['ms']:.0f} | {md_escape(summarize(r['excerpt']))} |"
        )
    out = os.path.join(DOCS_DIR, "ENDPOINT_TEST_REPORT.md")
    with open(out, "w") as f:
        f.write("\n".join(lines) + "\n")
    print(f"\n報告已寫入 {out}", file=sys.stderr)
    print(f"成功 {ok_count} / 錯誤 {err_count} / 共 {len(tools)}")


if __name__ == "__main__":
    main()
