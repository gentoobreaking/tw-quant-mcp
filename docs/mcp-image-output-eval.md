# MCP image 輸出改法評估（tw-quant-mcp）

> 日期：2026-08-01 ｜ 狀態：~~建議做~~ → **結論：不建議做**（2026-08-01 與 winvest.tw 比較後定案）
> 目標：讓 QClaw GUI 等 MCP 客戶端**直接看到 server 渲染的圖表**（不依賴 AI 解讀）

## 一、結論摘要

| 面向 | 評估 |
|---|---|
| 可行性 | ✅ MCP 協議原生支援 `image` content（base64 PNG） |
| SDK 支援 | ✅ go-sdk v1.7.0 有完整 `ImageContent`（`Data []byte` + `MimeType`） |
| 圖表庫 | ✅ go-chart v2 純 Go（freetype + x/image），**CGO-free 可維持** |
| 改動範圍 | 中：chart 套件 + Wire 層 + 少數 handler，約 300–500 行 |
| 相容性 | ✅ 向後相容（保留 text + structuredContent，額外附 image） |
| **價值比較** | **❌ 靜態 PNG 無法與 winvest.tw 等互動式商業圖表競爭 → 不值得做** |

**最終結論：不做。** 理由見「九、與 winvest.tw 比較後定案」。

## 二、現況（已查證）

- **MCP 協議**：`CallToolResult.Content` 支援 `text` / `image` / `audio` / `resource`；**無 `table` type**。
- **go-sdk v1.7.0**（`mcp/content.go`）：`ImageContent{ Data []byte; MimeType string }` 完整實作，`MarshalJSON` 正常。
- **tw-quant-mcp 現況**（`pkg/mcp/wire.go`）：成功回覆 = `TextContent`（JSON 字串）+ `StructuredContent`（Envelope）。**無 image**。
- **`_chart_meta` 已存在**：每個工具回傳建議圖表型態（line/bar/candlestick/heatmap），但 QClaw webchat 前端**不讀它畫圖** → 圖表資訊閒置。
- **QClaw/OpenClaw 對 image 的處理**：模型層有過濾 tool content 的 `image`（餵給 LLM）；webchat 前端對 tool result 的 image 渲染**未確認**（需實測）。

## 九、與 winvest.tw 比較後定案（2026-08-01）

使用者以 https://winvest.tw/Stock/Symbol/Comment/2308 為基準提問：
「這做出來的圖，有比 winvest 上的圖容易看麼？」

**比較結果（誠實評估）：**

| 面向 | winvest.tw | go-chart PNG |
|---|---|---|
| 互動性 | hover 看值、縮放、切期間 | 死的圖片 |
| 圖型 | 河流圖（紅/黃/藍色帶）、K線+量、多圖聯動 | 基本 line/bar/candlestick |
| 資訊密度 | 一頁整合 10+ 面向（健診/營收/EPS/股利/籌碼） | 單一圖一張 |
| 美觀 | 專業前端團隊 ECharts 級 | 預設樣式 |
| 成本 | 付費商業產品 | 半天工程換「堪用」 |

**定案理由：**
1. 靜態 PNG 先天輸給互動式商業圖表——做了也只是「有圖」而非「好看」
2. 真正的視覺化出路是互動 HTML（ECharts），但那是瀏覽器客戶端的場合，QClaw webchat 不是
3. MCP 的護城河是「對話中取得資料 + AI 解讀」，不是圖表美醜——資源應花在資料正確性與解讀品質
4. 若偶爾需要視覺化：由 AI 在對話中用 render_ui / embed / MEDIA 主動提供，不塞進 MCP 回傳

**若日後要視覺化的替代方向（不經 MCP image）：**
- AI 讀 `_chart_meta` → render_ui 互動面板（webchat）
- 互動 HTML（ECharts）+ 瀏覽器客戶端 embed
- 維持 JSON + AI 表格解讀（現況，已驗證）

## 十、附錄：原「建議做」內容保留供參考

以下為 2026-08-01 較早版本之評估（已定案不做，保留供日後參考）。

### 三、實作方案（原）

### 架構（最小侵入）

```
HandlerResult 新增欄位:
  ChartPNG []byte  // 可選：server 渲染好的 PNG

Core.Call 流程:
  handler 執行 → 若 ChartPNG 非空 → 附到回覆

Wire successResult:
  Content: [ TextContent(JSON), ImageContent(base64 PNG) ]
  StructuredContent: Envelope（不變，向後相容）
```

### 圖表產生

- 新增 `pkg/chart/render.go`：`RenderPNG(meta *chart.Meta, data any) ([]byte, error)`
  - 依 `_chart_meta.recommended_type` 分派：candlestick / line / bar / heatmap
  - 用 **go-chart v2**（純 Go，`go get github.com/wcharczuk/go-chart/v2`，依賴 freetype + x/image）
  - K 線圖：OHLC 資料 → 自繪蠟燭（go-chart 原生無 K 線，用 `ContinuousSeries` 組合或客製 renderer）
  - 中文標題：freetype 需載入中文字型（系統 `/System/Library/Fonts/PingFang.ttc` 或內嵌子集）——**這是主要工程點之一**

### 觸發方式（二選一，建議 A）

- **A. 參數觸發**：工具 schema 加 `chart_image: true` 參數 → 只有要求時才渲染圖（省成本、不破壞現有測試）
- **B. 恆常附帶**：所有工具一律附圖（簡單但增加 payload、拖慢回應）

### 適用工具（優先序）

| 優先 | 工具 | 圖型 |
|---|---|---|
| 1 | get_intraday_kline / get_stock_daily_kline | candlestick |
| 2 | get_stock_daily_quote | line（收盤價 + MA20/60） |
| 3 | get_put_call_ratio | line + hline 1.0 |
| 4 | get_market_summary | bar（漲跌家數） |
| 5 | get_foreign_industry_holdings | pie/bar |
| 6 | get_futures_daily_ohlc | candlestick |

## 四、改動清單

| 檔案 | 內容 | 估量 |
|---|---|---|
| `pkg/mcp/core.go` | HandlerResult 加 `ChartPNG []byte`；Call 回傳時附帶 | ~30 行 |
| `pkg/mcp/wire.go` | successResult 加 ImageContent | ~15 行 |
| `pkg/chart/render.go`（新） | RenderPNG：meta→go-chart→PNG bytes | ~200 行 |
| `pkg/chart/font.go`（新） | 中文字型載入（系統字型 fallback 鏈） | ~50 行 |
| 各 handler（選擇性） | K 線/行情工具產生 ChartPNG | ~15 行 × 6 |
| `go.mod` | + go-chart v2（純 Go） | — |
| 測試 | render_test（golden PNG）、wire_test（image content）、契約測試更新 | ~100 行 |

## 五、風險與對策

| 風險 | 對策 |
|---|---|
| **QClaw webchat 是否顯示 tool image 未實測**（最大未知） | 先做 5 行 POC：`curl` 直連 server 拿 image content，看 QClaw 反應；不顯示則改走「MEDIA 附件」或維持現狀 |
| 中文字型渲染 | 系統字型 fallback（macOS PingFang / Linux Noto / Windows 微軟正黑）；找不到字型則英文 fallback（不阻塞） |
| K 線無原生支援 | 客製 renderer 或 `ContinuousSeries` 組合（go-chart 社群常見做法） |
| 契約測試（Envelope 一致性） | image 附在 Content 而非 Envelope → 不破壞 `_lineage`/data 契約 |
| 效能 | 圖只在參數要求時渲染；PNG 快取（同快取鍵） |
| CGO-free 單一執行檔 | go-chart 純 Go，`CGO_ENABLED=0` 仍可建置（需驗證 freetype 無 cgo） |
| payload 變大 | base64 圖約 50–200KB/張；僅要求時附帶，可接受 |

## 六、驗收方式

1. `go build` + `go test ./...` + `go test -race ./...` 全綠
2. `CGO_ENABLED=0 go build` 成功（CGO-free 維持）
3. 新測試：`RenderPNG` 產出合法 PNG（magic bytes + 尺寸）；image content 序列化正確
4. **POC：QClaw 呼叫 → 看 GUI 是否顯示圖片**
   - 若顯示 → 完成「server 端渲染，GUI 直接看圖」
   - 若不顯示 → fallback：我讀結果後用 `MEDIA:` 貼圖（AI 渲染層）或做 embed gallery

## 七、替代方案（若不改 server）

| 方案 | 優點 | 缺點 |
|---|---|---|
| AI 讀結果 + `MEDIA:` 貼圖 | 零改動、立即見效 | 依賴 AI 判斷、每張圖都要生成 |
| `render_ui` 面板 | 互動式卡片 | 非標準 MCP、僅 webchat |
| 維持現狀（JSON + 我的表格） | 已驗證 | GUI 無原生視覺化 |

## 八、建議

1. **先做 5 分鐘 POC**（手動呼叫回 image 的測試 server → 觀察 QClaw GUI）——確認前端支援度，這決定整個方案的價值
2. POC 過 → 依「四、改動清單」實作，優先 K 線三工具
3. POC 不過 → 採「七、替代方案 1」（AI 貼圖），把渲染腳本（render_tool_snapshots.py）掛進流程即可
