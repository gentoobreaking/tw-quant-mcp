# tw-quant-mcp 自動化快取刷新機制

## 概述

本文件說明 `tw-quant-mcp` 專案的自動化快取刷新機制，確保財報、月營收、股利等資料在官方更新後能自動失效重抓，不需人工介入。

---

## 架構設計

```
Host (macOS/Linux)
├── cron (排程觸發)
│   ├── 每天 06:00  → auto_cache_refresh.py (自動判斷)
│   ├── 每月 11 號 07:00  → --force-monthly
│   ├── 財報截止日 07:00  → --force-financials
│   └── 每月 1,15 號 07:00  → --force-dividend
├── /Users/david/Projects/tw-quant-mcp/
│   ├── data/cache.db          ← 共享 L2 SQLite 快取
│   ├── scripts/auto_cache_refresh.py
│   └── logs/cache_refresh.log
└── 其他專案 (tw-quant-pickup 等)
    └── docker-compose.yml 掛載 ../tw-quant-mcp/data:/app/mcp_data:ro
```

**關鍵原則**：
- ✅ 腳本放在 `tw-quant-mcp` 專案，由**宿主機 cron** 執行
- ✅ 不依賴 container 存活狀態
- ✅ 多專案共用同一 `cache.db`
- ✅ 修改腳本即時生效，無需重建 image

---

## 快取 TTL 政策 (pkg/cache/policy.go)

| 資料類型 | 官方更新頻率 | 系統 TTL | 關鍵刷新時間點 |
|----------|-------------|----------|----------------|
| **月營收** | 每月 10 日前 | **30 天** | 每月 11 號 |
| **財報三表** | 每季截止 (5/15, 8/14, 11/14, 3/31) | **90 天** | 截止日當天 |
| **獲利能力指標** | 配合財報 | **90 天** | 截止日當天 |
| **完整財報 (AJAX)** | 配合財報 | **90 天** | 截止日當天 |
| **股利分派** | 每年 5-6 月密集 | **12 小時** | 每月 1, 15 號檢查 |
| **除權息行事曆** | 隨時公告 | **6 小時** | 每月 1, 15 號檢查 |
| **重大訊息** | 隨時 | **5 分鐘** | 高頻自動輪詢 |

---

## 自動化腳本

### 位置
```
/Users/david/Projects/tw-quant-mcp/scripts/auto_cache_refresh.py
```

### 使用方式

```bash
# 自動模式：依目前日期判斷關鍵時間點
python3 scripts/auto_cache_refresh.py

# 強制模式
python3 scripts/auto_cache_refresh.py --force-monthly      # 清月營收
python3 scripts/auto_cache_refresh.py --force-financials   # 清財報
python3 scripts/auto_cache_refresh.py --force-dividend     # 清股利/除權息
python3 scripts/auto_cache_refresh.py --all                # 清全部

# 試運行 (不實際刪除)
python3 scripts/auto_cache_refresh.py --force-monthly --dry-run
```

### 清除規則對照

| 參數 | 清除 Pattern | 適用場景 |
|------|-------------|----------|
| `--force-monthly` | `%monthly_revenue%` | 月營收更新 |
| `--force-financials` | `%financials%`, `%income_summary%`, `%profit_ratios%`, `%balance_sheet%`, `%cash_flow%`, `%income_statement%` | 財報截止日 |
| `--force-dividend` | `%dividend%`, `%ex_div_calendar%` | 除權息前檢查 |
| `--all` | `%` | 緊急完全重建 |

---

## Crontab 設定

```bash
# 編輯 crontab
crontab -e

# ================================
# tw-quant-mcp 自動快取刷新排程
# ================================

# 每天 06:00 - 自動判斷模式
0 6 * * * /opt/homebrew/bin/python3 /Users/david/Projects/tw-quant-mcp/scripts/auto_cache_refresh.py >> /Users/david/Projects/tw-quant-mcp/logs/cache_refresh.log 2>&1

# 每月 11 號 07:00 - 月營收雙重保險
0 7 11 * * /opt/homebrew/bin/python3 /Users/david/Projects/tw-quant-mcp/scripts/auto_cache_refresh.py --force-monthly >> /Users/david/Projects/tw-quant-mcp/logs/cache_refresh.log 2>&1

# 財報截止日 07:00 (Q1: 5/15, Q2: 8/14, Q3: 11/14, Q4: 3/31)
0 7 15 5 * /opt/homebrew/bin/python3 /Users/david/Projects/tw-quant-mcp/scripts/auto_cache_refresh.py --force-financials >> /Users/david/Projects/tw-quant-mcp/logs/cache_refresh.log 2>&1
0 7 14 8 * /opt/homebrew/bin/python3 /Users/david/Projects/tw-quant-mcp/scripts/auto_cache_refresh.py --force-financials >> /Users/david/Projects/tw-quant-mcp/logs/cache_refresh.log 2>&1
0 7 14 11 * /opt/homebrew/bin/python3 /Users/david/Projects/tw-quant-mcp/scripts/auto_cache_refresh.py --force-financials >> /Users/david/Projects/tw-quant-mcp/logs/cache_refresh.log 2>&1
0 7 31 3 * /opt/homebrew/bin/python3 /Users/david/Projects/tw-quant-mcp/scripts/auto_cache_refresh.py --force-financials >> /Users/david/Projects/tw-quant-mcp/logs/cache_refresh.log 2>&1

# 每月 1, 15 號 07:00 - 除權息/股利檢查
0 7 1,15 * * /opt/homebrew/bin/python3 /Users/david/Projects/tw-quant-mcp/scripts/auto_cache_refresh.py --force-dividend >> /Users/david/Projects/tw-quant-mcp/logs/cache_refresh.log 2>&1
```

### 驗證 crontab

```bash
# 查看目前排程
crontab -l

# 手動測試
/opt/homebrew/bin/python3 /Users/david/Projects/tw-quant-mcp/scripts/auto_cache_refresh.py --dry-run
```

---

## 多專案共用設定

### tw-quant-pickup / docker-compose.yml

```yaml
services:
  mcp-server:
    # 唯一寫入 cache.db 的服務
    environment:
      - DATA_DIR=/app/data
    volumes:
      - ../tw-quant-mcp/data:/app/data

  api-server:
    # 唯讀共享快取
    volumes:
      - ../tw-quant-mcp/data:/app/mcp_data:ro

  scheduler:
    # 唯讀共享快取
    volumes:
      - ../tw-quant-mcp/data:/app/mcp_data:ro
```

### 其他專案參考

```yaml
# 任何需要呼叫 tw-quant-mcp 的專案
services:
  your-service:
    volumes:
      - /Users/david/Projects/tw-quant-mcp/data:/app/mcp_data:ro
    environment:
      - MCP_TRANSPORT=streamable-http
      - MCP_HTTP_ADDR=http://mcp-server:8000  # 或 host.docker.internal:8000
```

---

## 日誌與監控

### 日誌位置
```
/Users/david/Projects/tw-quant-mcp/logs/cache_refresh.log
```

### 日誌範例

```
2026-08-21 06:00:00 [INFO] 今天無需刷新快取 (2026-08-21)
2026-08-21 06:00:00 [INFO] 完成：共刪除 0 筆快取

2026-09-11 06:00:00 [INFO] Auto refresh triggered: monthly revenue (11th)
2026-09-11 06:00:00 [INFO] Deleted 1247 entries matching: %monthly_revenue%
2026-09-11 06:00:00 [INFO] 完成：共刪除 1247 筆快取

2026-11-14 07:00:00 [INFO] Auto refresh triggered: financials deadline (11/14)
2026-11-14 07:00:00 [INFO] Deleted 3421 entries matching: %financials%
2026-11-14 07:00:00 [INFO] Deleted 2891 entries matching: %income_summary%
2026-11-14 07:00:00 [INFO] Deleted 2891 entries matching: %profit_ratios%
2026-11-14 07:00:00 [INFO] 完成：共刪除 9203 筆快取
```

### 監控建議

```bash
# 設定 logrotate (避免日誌過大)
cat > /etc/logrotate.d/tw-quant-mcp << 'EOF'
/Users/david/Projects/tw-quant-mcp/logs/*.log {
    daily
    rotate 30
    compress
    delaycompress
    missingok
    notifempty
}
EOF
```

---

## 故障排查

### 1. 快取未生效

```bash
# 檢查 cache.db 是否存在
ls -la /Users/david/Projects/tw-quant-mcp/data/cache.db

# 檢查快取內容
sqlite3 /Users/david/Projects/tw-quant-mcp/data/cache.db \
  "SELECT key, datetime(created_at/1000000000, 'unixepoch') as created FROM cache_entries WHERE key LIKE '%monthly_revenue%' LIMIT 5;"
```

### 2. cron 未執行

```bash
# 檢查 cron 服務
launchctl list | grep cron  # macOS
systemctl status cron       # Linux

# 檢查 cron 日誌
grep CRON /var/log/syslog   # Linux
log show --predicate 'subsystem == "com.apple.cron"' --last 1h  # macOS
```

### 3. 權限問題

```bash
# 確保腳本可執行
chmod +x /Users/david/Projects/tw-quant-mcp/scripts/auto_cache_refresh.py

# 確保 logs 目錄可寫
ls -ld /Users/david/Projects/tw-quant-mcp/logs/
```

### 4. Python 路徑問題

```bash
# 確認 python3 路徑 (crontab 使用完整路徑)
which python3
# /opt/homebrew/bin/python3  (Apple Silicon macOS)
# /usr/bin/python3           (Linux)

# crontab 中使用完整路徑
0 6 * * * /opt/homebrew/bin/python3 /Users/david/Projects/tw-quant-mcp/scripts/auto_cache_refresh.py ...
```

---

## 相關檔案清單

| 檔案 | 說明 |
|------|------|
| `scripts/auto_cache_refresh.py` | 主自動化腳本 |
| `scripts/sync_symbol_registry.py` | Symbol Registry 同步 (T036) |
| `pkg/cache/policy.go` | TTL 政策定義 |
| `pkg/registry/loader.go` | Symbol Registry 載入 (含 manual_overrides) |
| `data/manual_overrides.json` | 手動補齊代碼 (6518, 0050 等) |
| `logs/cache_refresh.log` | 執行日誌 |

---

## 版本歷程

| 版本 | 日期 | 變更 |
|------|------|------|
| 1.0 | 2026-08-21 | 初版：建立自動化腳本、crontab 設定、多專案共用架構 |

---

## 維護者注意事項

1. **官方更新時間若有變動**，請同步更新腳本中的關鍵日期判斷邏輯
2. **新增資料類型**時，需在 `PATTERNS` 字典與 `pkg/cache/policy.go` 同步新增
3. **容器化部署**時，確保宿主機 cron 正常運行，container 僅負責服務不負責排程
4. **多專案共用**時，cache.db 掛載權限建議 `:ro` (唯讀)，僅 mcp-server 掛載可寫入