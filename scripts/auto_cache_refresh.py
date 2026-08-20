#!/usr/bin/env python3
"""
tw-quant-mcp 自動快取刷新腳本
由宿主機 cron 執行，不依賴 container 狀態

Usage:
    python scripts/auto_cache_refresh.py              # 依日期自動判斷
    python scripts/auto_cache_refresh.py --force-monthly
    python scripts/auto_cache_refresh.py --force-financials
    python scripts/auto_cache_refresh.py --force-dividend
    python scripts/auto_cache_refresh.py --all
"""

import argparse
import logging
import sqlite3
import sys
from datetime import datetime
from pathlib import Path

# 專案根目錄
PROJECT_ROOT = Path(__file__).parent.parent
CACHE_DB = PROJECT_ROOT / "data" / "cache.db"
LOG_DIR = PROJECT_ROOT / "logs"
LOG_DIR.mkdir(exist_ok=True)

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s [%(levelname)s] %(message)s",
    handlers=[
        logging.FileHandler(LOG_DIR / "cache_refresh.log"),
        logging.StreamHandler(sys.stdout),
    ],
)
log = logging.getLogger(__name__)

# 快取清除規則
PATTERNS = {
    "monthly": ["%monthly_revenue%"],
    "financials": ["%financials%", "%income_summary%", "%profit_ratios%", "%balance_sheet%", "%cash_flow%", "%income_statement%"],
    "dividend": ["%dividend%", "%ex_div_calendar%"],
    "all": ["%"],
}

def clean_patterns(patterns: list[str], dry_run: bool = False) -> int:
    """清除符合 pattern 的快取條目"""
    if not CACHE_DB.exists():
        log.error("Cache DB 不存在: %s", CACHE_DB)
        return 0
    
    total_deleted = 0
    try:
        conn = sqlite3.connect(CACHE_DB)
        cursor = conn.cursor()
        
        for pattern in patterns:
            if dry_run:
                cursor.execute("SELECT COUNT(*) FROM cache_entries WHERE key LIKE ?", (pattern,))
                count = cursor.fetchone()[0]
                log.info("[DRY-RUN] Would delete %d entries matching: %s", count, pattern)
            else:
                cursor.execute("DELETE FROM cache_entries WHERE key LIKE ?", (pattern,))
                deleted = cursor.rowcount
                total_deleted += deleted
                log.info("Deleted %d entries matching: %s", deleted, pattern)
        
        if not dry_run:
            conn.commit()
        conn.close()
        
    except Exception as e:
        log.exception("Clean failed: %s", e)
        raise
    
    return total_deleted

def auto_refresh() -> int:
    """依目前日期自動判斷要清除的快取"""
    now = datetime.now()
    patterns = []
    reasons = []
    
    # 每月 11 號：月營收
    if now.day == 11:
        patterns.extend(PATTERNS["monthly"])
        reasons.append("monthly revenue (11th)")
    
    # 財報截止日
    if (now.month, now.day) in [(5, 15), (8, 14), (11, 14), (3, 31)]:
        patterns.extend(PATTERNS["financials"])
        reasons.append(f"financials deadline ({now.month}/{now.day})")
    
    # 除權息前兩週 (簡化：每月 1, 15 號檢查)
    if now.day in [1, 15]:
        patterns.extend(PATTERNS["dividend"])
        reasons.append("dividend/ex-div check")
    
    if not patterns:
        log.info("今天無需刷新快取 (%s)", now.strftime("%Y-%m-%d"))
        return 0
    
    log.info("Auto refresh triggered: %s", ", ".join(reasons))
    return clean_patterns(patterns)

def main():
    parser = argparse.ArgumentParser(description="tw-quant-mcp Cache Auto Refresh")
    parser.add_argument("--force-monthly", action="store_true", help="強制刷月營收快取")
    parser.add_argument("--force-financials", action="store_true", help="強制刷財報快取")
    parser.add_argument("--force-dividend", action="store_true", help="強制刷股利/除權息快取")
    parser.add_argument("--all", action="store_true", help="清除所有快取")
    parser.add_argument("--dry-run", action="store_true", help="只顯示會刪除什麼，不實際執行")
    args = parser.parse_args()
    
    if args.all:
        patterns = PATTERNS["all"]
    elif args.force_monthly:
        patterns = PATTERNS["monthly"]
    elif args.force_financials:
        patterns = PATTERNS["financials"]
    elif args.force_dividend:
        patterns = PATTERNS["dividend"]
    else:
        return auto_refresh()
    
    return clean_patterns(patterns, dry_run=args.dry_run)

if __name__ == "__main__":
    try:
        deleted = main()
        log.info("完成：共刪除 %d 筆快取", deleted)
        sys.exit(0)
    except Exception as e:
        log.exception("執行失敗: %s", e)
        sys.exit(1)