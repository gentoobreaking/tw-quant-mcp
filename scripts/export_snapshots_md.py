#!/usr/bin/env python3
"""將 snapshots/raw/*.json（全部工具真實呼叫結果）匯出為單一 Markdown 報告。

用法:
  python3 scripts/export_snapshots_md.py [輸出檔路徑]

輸出預設: snapshots/REPORT.md
可與 make snapshots 搭配：先呼叫工具 → 再匯出報告。
"""
import json
import os
import sys
from datetime import datetime

PROJ = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
RAW_DIR = os.path.join(PROJ, "snapshots", "raw")
DEFAULT_OUT = os.path.join(PROJ, "snapshots", "REPORT.md")


def extract_result(obj):
    """相容兩種儲存格式：批次 {response:{result}} / one_tool 舊格式 {result}"""
    if isinstance(obj.get("response"), dict):
        return obj["response"].get("result") or {}
    if isinstance(obj.get("result"), dict):
        return obj["result"]
    return {}


def fmt_value(v, depth=0):
    """把值轉成 markdown 表格可用的字串；太深/太長就摘要。"""
    if isinstance(v, dict):
        if depth >= 2 or len(json.dumps(v, ensure_ascii=False)) > 200:
            return f"`{json.dumps(v, ensure_ascii=False)[:180]}…`"
        return ", ".join(f"{k}={fmt_value(x, depth+1)}" for k, x in v.items())
    if isinstance(v, list):
        if len(v) == 0:
            return "`[]`"
        if len(v) == 1:
            return fmt_value(v[0], depth+1)
        return f"`[{len(v)} 筆] 例: {fmt_value(v[0], depth+1)[:120]}…`"
    if isinstance(v, str):
        s = v if len(v) <= 80 else v[:80] + "…"
        return s
    return str(v)


def data_to_markdown(data, max_rows=12):
    """data 區塊：dict 轉表格（嵌套值摘要）、list 轉代碼塊（截斷）。"""
    if isinstance(data, dict):
        rows = []
        for k, v in list(data.items())[:max_rows]:
            rows.append(f"| {k} | {fmt_value(v)} |")
        if len(data) > max_rows:
            rows.append(f"| … | 共 {len(data)} 鍵（其餘省略） |")
        return "| 欄位 | 值 |\n|---|---|\n" + "\n".join(rows)
    if isinstance(data, list):
        if len(data) == 0:
            return "`[]`（無資料）"
        text = json.dumps(data, ensure_ascii=False, indent=2)
        if len(text) > 4000:
            text = text[:4000] + "\n  …（截斷）"
        return f"```json\n{text}\n```"
    return f"`{fmt_value(data)}`"


def lineage_to_markdown(lineage):
    if not isinstance(lineage, dict):
        return f"`{lineage}`"
    rows = [f"| {k} | {fmt_value(v)} |" for k, v in lineage.items()]
    return "| 欄位 | 值 |\n|---|---|\n" + "\n".join(rows)


def build_report(files):
    lines = []
    ok_count = 0
    err_count = 0
    for name in sorted(files):
        try:
            with open(os.path.join(RAW_DIR, name), encoding="utf-8") as fh:
                obj = json.load(fh)
        except (OSError, json.JSONDecodeError) as e:
            print(f"略過無法解析的 {name}: {e}", file=sys.stderr)
            continue
        result = extract_result(obj)
        is_err = result.get("isError", False)
        content = result.get("content") or []
        text = "\n".join(c.get("text", "") for c in content if c.get("type") == "text")
        try:
            parsed = json.loads(text) if text else {}
        except Exception:
            parsed = None

        if is_err:
            err_count += 1
        else:
            ok_count += 1

        args = obj.get("arguments", {})
        badge = "❌" if is_err else "✅"
        lines.append(f"## {name} {badge}")
        lines.append("")
        lines.append(f"- **參數**: `{json.dumps(args, ensure_ascii=False)}`")

        if is_err:
            lines.append(f"- **錯誤**: {text[:400]}")
            lines.append("")
            continue

        if parsed:
            data = parsed.get("data")
            lineage = parsed.get("_lineage", {})
            chart = parsed.get("_chart_meta")
            http_calls = parsed.get("http_calls", "?")
            disclaimer = parsed.get("disclaimer", "")
            lines.append(f"- **http_calls**: {http_calls}")
            if chart:
                lines.append(f"- **建議圖表**: `{chart.get('recommended_type', '?')}`")
            lines.append("")
            lines.append("### data")
            lines.append("")
            lines.append(data_to_markdown(data))
            lines.append("")
            lines.append("### _lineage")
            lines.append("")
            lines.append(lineage_to_markdown(lineage))
            if disclaimer:
                lines.append("")
                lines.append(f"> {disclaimer}")
        else:
            lines.append(f"- **回應**: {text[:400]}")
        lines.append("")

    # 報告頭
    header = [
        "# tw-quant-mcp 全部工具真實呼叫報告",
        "",
        f"- 產生時間: {datetime.now().strftime('%Y-%m-%d %H:%M %Z')}",
        f"- 工具總數: {ok_count + err_count}",
        f"- ✅ 成功: {ok_count}",
        f"- ❌ 錯誤: {err_count}",
        "",
        "> 資料來源為官方公開免費資料（TWSE/TPEx/MOPS/TAIFEX），僅供研究參考，不構成投資建議。",
        "",
        "---",
        "",
    ]
    return "\n".join(header + lines)


def main():
    out = sys.argv[1] if len(sys.argv) > 1 else DEFAULT_OUT
    try:
        files = sorted(f for f in os.listdir(RAW_DIR) if f.endswith(".json"))
    except OSError as e:
        print(f"無法讀取快照目錄 {RAW_DIR}: {e}", file=sys.stderr)
        sys.exit(1)
    if not files:
        print(f"❌ {RAW_DIR} 沒有 JSON 結果，請先執行 make snapshots 或 make snapshots-call")
        sys.exit(1)
    report = build_report(files)
    try:
        with open(out, "w", encoding="utf-8") as fh:
            fh.write(report)
    except OSError as e:
        print(f"寫入報告失敗: {e}", file=sys.stderr)
        sys.exit(1)
    # 統計（依工具數，不依符號出現次數）
    ok_count = 0
    err_count = 0
    for f in files:
        try:
            with open(os.path.join(RAW_DIR, f), encoding="utf-8") as fh:
                obj = json.load(fh)
        except (OSError, json.JSONDecodeError) as e:
            print(f"略過無法解析的 {f}: {e}", file=sys.stderr)
            continue
        result = extract_result(obj)
        if result.get("isError", False):
            err_count += 1
        else:
            ok_count += 1
    print(f"✅ 已匯出: {out}")
    print(f"   工具 {ok_count + err_count} 個（成功 {ok_count} / 錯誤 {err_count}）")


if __name__ == "__main__":
    main()
