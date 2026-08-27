#!/usr/bin/env python3
"""單一工具呼叫：對 tw-quant-mcp 呼叫指定工具，結果存到 snapshots/raw/。

用法:
  python3 one_tool.py <工具名> ['<JSON 參數>']

範例:
  python3 one_tool.py get_margin_trading '{"symbol":"2330"}'
  python3 one_tool.py get_financial_statements '{"symbol":"1232"}'
  python3 one_tool.py get_stock_daily_quote '{"symbol":"2330"}'

若省略參數，會嘗試從 call_tw_quant_tools.py 的 ARGS 表自動帶入。
輸出: snapshots/raw/<工具名>.json（格式與批次腳本一致，可被 render_tool_snapshots.py 渲染）
"""

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

# 從批次腳本匯入 ARGS 表（工具名 → 預設參數）
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
try:
    from call_tw_quant_tools import ARGS as DEFAULT_ARGS
except Exception:
    DEFAULT_ARGS = {}


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
        except json.JSONDecodeError:
            continue  # 非 JSON 行，忽略
        if obj.get("id") == msg_id:
            return obj
    return {"jsonrpc": "2.0", "id": msg_id, "error": {"message": "timeout"}}


def main():
    if len(sys.argv) < 2:
        print(__doc__)
        sys.exit(1)
    tool_name = sys.argv[1]
    if len(sys.argv) >= 3:
        try:
            args = json.loads(sys.argv[2])
        except json.JSONDecodeError:
            print(f"參數 JSON 解析失敗: {sys.argv[2]}")
            sys.exit(1)
    else:
        args = DEFAULT_ARGS.get(tool_name, {})
        print(
            f"（未提供參數，使用 ARGS 表預設: {json.dumps(args, ensure_ascii=False)}）"
        )

    if not os.path.exists(BIN):
        print(f"執行檔不存在: {BIN}\n請先執行 make build 或 make snapshots")
        sys.exit(1)

    proc = subprocess.Popen(
        [BIN],
        stdin=subprocess.PIPE,
        stdout=subprocess.PIPE,
        stderr=subprocess.DEVNULL,
        text=True,
        bufsize=1,
    )
    # initialize 握手
    init = {
        "jsonrpc": "2.0",
        "id": 1,
        "method": "initialize",
        "params": {
            "protocolVersion": "2025-11-25",
            "capabilities": {},
            "clientInfo": {"name": "one-tool-cli", "version": "1.0.0"},
        },
    }
    if proc.stdin is None or proc.stdout is None:
        print("無法建立 stdio 管線", file=sys.stderr)
        sys.exit(1)
    proc.stdin.write(json.dumps(init) + "\n")
    proc.stdin.flush()
    deadline = time.time() + 15
    while time.time() < deadline:
        line = proc.stdout.readline()
        if line:
            try:
                if json.loads(line).get("id") == 1:
                    break
            except json.JSONDecodeError:
                continue  # 非 JSON 行，忽略
    proc.stdin.write('{"jsonrpc":"2.0","method":"notifications/initialized"}\n')
    proc.stdin.flush()

    print(f"呼叫 {tool_name} ...", flush=True)
    resp = call_tool(proc, tool_name, args, 10)
    proc.terminate()

    # 判斷結果（MCP 錯誤 vs 工具錯誤）
    if "error" in resp:
        print(f"❌ MCP 錯誤: {resp['error'].get('message')}")
        result_obj = {
            "isError": True,
            "content": [{"type": "text", "text": json.dumps(resp["error"])}],
        }
    else:
        result = resp.get("result", {})
        is_err = result.get("isError", False)
        texts = [
            c.get("text", "")
            for c in result.get("content", [])
            if c.get("type") == "text"
        ]
        text = "\n".join(texts)
        status = "❌ 工具錯誤" if is_err else "✅ 成功"
        print(f"{status} (isError={is_err})")
        if is_err:
            print(f"錯誤訊息: {text[:500]}")
        else:
            try:
                parsed = json.loads(text)
                print(
                    f"data 摘要: {json.dumps(parsed.get('data'), ensure_ascii=False)[:300]}"
                )
                print(f"_lineage.source: {parsed.get('_lineage', {}).get('source')}")
            except Exception:
                print(f"回應前 300 字: {text[:300]}")
        result_obj = {"isError": is_err, "content": result.get("content", [])}

    out = {
        "arguments": args,
        "response": {"result": result_obj},
    }
    path = os.path.join(RAW_DIR, f"{tool_name}.json")
    try:
        with open(path, "w", encoding="utf-8") as f:
            json.dump(out, f, ensure_ascii=False, indent=2)
        print(f"已存: {path}")
    except OSError as e:
        print(f"寫檔失敗: {e}", file=sys.stderr)
        sys.exit(1)
    print("提示: make snapshots-render 可重新渲染所有 PNG")


if __name__ == "__main__":
    main()
