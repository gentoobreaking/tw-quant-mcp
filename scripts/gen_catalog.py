#!/usr/bin/env python3
"""由 tools/list JSON 產生 docs/TOOL_CATALOG.md。

用法：gen_catalog.py <version> <tools_json>
（version 僅供頁首標註；tools_json 為 tools/list result.tools 的 JSON 字串）
輸出寫到 stdout。
"""

import json
import sys


def main() -> None:
    version = sys.argv[1]
    try:
        tools = sorted(json.loads(sys.argv[2]), key=lambda t: t["name"])
    except (json.JSONDecodeError, KeyError, IndexError) as exc:
        sys.exit(f"gen_catalog: tools/list JSON 解析失敗: {exc}")

    lines = [
        f"# 附錄：完整工具目錄（{len(tools)} 個）",
        "",
        f"> 由真實服務 `tools/list` 輸出自動彙出（v{version}）；更新方式：`make catalog`。",
        "> 各工具之 Envelope、`_lineage`、快取政策與真實呼叫快照見",
        "> `snapshots/raw/<tool>.json`。",
        "> 逐源覆蓋分析見 `docs/TOOL_COVERAGE_BY_SOURCE.md`；與遠端 TWSEMCPServer",
        "> 的對照見 `docs/COMPARISON_TWSEMCPServer.md`。",
        "",
    ]
    for t in tools:
        desc = " ".join((t.get("description") or "").split())
        lines.append(f"- `{t['name']}`：{desc}")

    sys.stdout.write("\n".join(lines) + "\n")


if __name__ == "__main__":
    main()
