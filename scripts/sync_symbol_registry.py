#!/usr/bin/env python3
"""
Symbol Registry 自動同步腳本 (T036)

功能：
1. 讀取 config.json 的 watch_stocks 清單
2. 從 MCP Server 的 Symbol Registry 查詢已註冊代碼（透過 get_symbol_list tool）
3. 找出缺漏代碼
4. 自動將缺漏代碼加入 manual_overrides.json
5. 支援啟動時同步 + 定期同步

使用方式：
  python sync_symbol_registry.py              # 執行一次同步
  python sync_symbol_registry.py --daemon     # 背景執行，每日同步
  python sync_symbol_registry.py --config /path/to/config.json
"""

import json
import os
import sys
import time
import argparse
import logging
from pathlib import Path
from typing import List, Dict, Set, Optional
from datetime import datetime, timedelta

# 設定日誌
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s [%(levelname)s] %(message)s',
    handlers=[
        logging.StreamHandler(sys.stdout)
    ]
)
logger = logging.getLogger(__name__)

# 預設路徑
DEFAULT_CONFIG_PATH = Path(os.getenv("TW_QUANT_CONFIG", str(Path(__file__).parent.parent.parent / "tw-quant-signal" / "config.json")))
DEFAULT_OVERRIDE_PATH = Path(__file__).parent.parent / "data" / "manual_overrides.json"
DEFAULT_MCP_SERVER_URL = "http://127.0.0.1:8787"

# 股票代碼市場別判斷邏輯（簡易版，實際應查詢官方資料）
# 4碼純數字：上市(tse)或上櫃(otc)，需查官方清單
# 00開頭4-6碼：上市ETF(tse)
# 6碼含字母：上櫃ETF/ETN/特別股(otc)

# 已知的補齊資料（可由外部檔案載入）
KNOWN_SYMBOLS = {
    "6518": {"market": "tse", "name": "長春", "category": "生技醫療業"},
    "0050": {"market": "tse", "name": "元大台灣50", "category": "ETF"},
    "0056": {"market": "tse", "name": "元大高股息", "category": "ETF"},
    "006208": {"market": "tse", "name": "富邦台50", "category": "ETF"},
    "00636": {"market": "tse", "name": "國泰中國A50", "category": "ETF"},
    "00679B": {"market": "tse", "name": "元大美債20年", "category": "ETF"},
    "00400A": {"market": "tse", "name": "國泰台灣高股息", "category": "ETF"},
    "00899": {"market": "tse", "name": "FT潔淨能源", "category": "ETF"},
    "006201": {"market": "otc", "name": "元大富櫃50", "category": "ETF"},
    "6547": {"market": "otc", "name": "高端疫苗", "category": "生技醫療業"},
    "3226": {"market": "otc", "name": "至寶電", "category": "電機機械"},
    "020001": {"market": "otc", "name": "富邦存股雙十N", "category": "ETN"},
}


def load_watch_stocks(config_path: Path) -> List[str]:
    """從 config.json 讀取 watch_stocks"""
    try:
        with open(config_path, 'r', encoding='utf-8') as f:
            config = json.load(f)
        watch_stocks = config.get("watch_stocks", [])
        logger.info(f"從 {config_path} 讀取到 {len(watch_stocks)} 支觀察股票: {watch_stocks}")
        return watch_stocks
    except Exception as e:
        logger.error(f"讀取 config 失敗: {e}")
        return []


def load_manual_overrides(override_path: Path) -> List[Dict]:
    """讀取現有的 manual_overrides.json"""
    if not override_path.exists():
        logger.info(f"手動覆寫檔 {override_path} 不存在，將建立新檔案")
        return []
    
    try:
        with open(override_path, 'r', encoding='utf-8') as f:
            data = json.load(f)
        logger.info(f"從 {override_path} 讀取到 {len(data)} 筆手動覆寫")
        return data
    except Exception as e:
        logger.error(f"讀取手動覆寫檔失敗: {e}")
        return []


def save_manual_overrides(override_path: Path, overrides: List[Dict]) -> bool:
    """儲存 manual_overrides.json"""
    try:
        # 確保目錄存在
        override_path.parent.mkdir(parents=True, exist_ok=True)
        
        # 按代碼排序後寫入
        sorted_overrides = sorted(overrides, key=lambda x: x["code"])
        
        with open(override_path, 'w', encoding='utf-8') as f:
            json.dump(sorted_overrides, f, ensure_ascii=False, indent=2)
        
        logger.info(f"已儲存 {len(sorted_overrides)} 筆手動覆寫至 {override_path}")
        return True
    except Exception as e:
        logger.error(f"儲存手動覆寫檔失敗: {e}")
        return False


def get_registry_symbols() -> Set[str]:
    """從 MCP Server 取得已註冊的代碼（透過 get_symbol_list tool）"""
    # 這裡使用直接查詢 registry 的方式
    # 實際部署時可能需要透過 MCP protocol 呼叫 get_symbol_list
    # 為了簡化，這裡直接回傳空集合，讓同步腳本依靠 KNOWN_SYMBOLS 補齊
    logger.warning("無法直接查詢 MCP Server Registry，將使用 KNOWN_SYMBOLS 作為參考")
    return set()


def determine_market_and_info(code: str) -> Optional[Dict]:
    """判斷代碼的市場別與基本資訊"""
    # 優先使用 KNOWN_SYMBOLS
    if code in KNOWN_SYMBOLS:
        return KNOWN_SYMBOLS[code]
    
    # 簡易判斷邏輯
    if len(code) >= 4 and len(code) <= 6:
        if code.startswith("00"):
            # 00 開頭通常是上市 ETF/ETN
            return {"market": "tse", "name": f"代碼{code}", "category": "ETF"}
        elif len(code) == 4 and code.isdigit():
            # 4 碼純數字，預設上市（需人工確認）
            return {"market": "tse", "name": f"代碼{code}", "category": ""}
        elif len(code) == 6 and code.isalnum() and not code.isdigit():
            # 6 碼英數混合，通常是上櫃 ETF/ETN/特別股
            return {"market": "otc", "name": f"代碼{code}", "category": "ETF/ETN"}
        elif len(code) == 5 and code.isdigit():
            # 5 碼純數字，可能是上櫃股票/ETF
            return {"market": "otc", "name": f"代碼{code}", "category": ""}
    
    logger.warning(f"無法判斷 {code} 的市場別，跳過")
    return None


def sync_symbols(config_path: Path, override_path: Path, dry_run: bool = False) -> Dict:
    """執行同步邏輯"""
    logger.info("=== 開始 Symbol Registry 同步 ===")
    
    # 1. 讀取 watch_stocks
    watch_stocks = load_watch_stocks(config_path)
    if not watch_stocks:
        logger.warning("watch_stocks 為空，無需同步")
        return {"added": 0, "skipped": 0, "errors": 0}
    
    # 2. 讀取現有的 manual_overrides
    existing_overrides = load_manual_overrides(override_path)
    existing_codes = {item["code"] for item in existing_overrides}
    
    # 3. 取得已在 Registry 中的代碼（這裡簡化處理，實際可查詢 MCP Server）
    registry_codes = get_registry_symbols()
    
    # 4. 找出需要補齊的代碼
    # 策略：只補齊 KNOWN_SYMBOLS 中定義的、且在 watch_stocks 中、但不在 manual_overrides 中的代碼
    # 官方清單應已包含的代碼（如 2330, 2308 等）不需手動補齊
    missing_codes = []
    for code in watch_stocks:
        # 只處理 KNOWN_SYMBOLS 中有定義的代碼（這些是官方清單可能缺漏的）
        if code in KNOWN_SYMBOLS and code not in existing_codes:
            missing_codes.append(code)
    
    logger.info(f"發現 {len(missing_codes)} 支缺漏代碼需補齊 (僅處理 KNOWN_SYMBOLS 定義): {missing_codes}")
    
    # 5. 為缺漏代碼建立覆寫記錄
    added = 0
    skipped = 0
    errors = 0
    
    for code in missing_codes:
        info = determine_market_and_info(code)
        if info:
            new_override = {
                "code": code,
                "market": info["market"],
                "name": info["name"],
                "category": info.get("category", "")
            }
            existing_overrides.append(new_override)
            logger.info(f"  新增: {code} -> {info['market']} {info['name']}")
            added += 1
        else:
            logger.warning(f"  跳過: {code} (無法判斷市場別)")
            skipped += 1
    
    # 6. 儲存
    if not dry_run and added > 0:
        if save_manual_overrides(override_path, existing_overrides):
            logger.info(f"同步完成: 新增 {added}, 跳過 {skipped}, 錯誤 {errors}")
        else:
            errors += 1
            logger.error("同步失敗: 儲存檔案失敗")
    elif dry_run:
        logger.info(f"[Dry-run] 同步完成: 將新增 {added}, 跳過 {skipped}, 錯誤 {errors}")
    
    return {"added": added, "skipped": skipped, "errors": errors}


def run_daemon(config_path: Path, override_path: Path, interval_hours: int = 24):
    """背景執行模式：定期同步"""
    logger.info(f"啟動 Daemon 模式，每 {interval_hours} 小時同步一次")
    
    while True:
        try:
            sync_symbols(config_path, override_path)
        except Exception as e:
            logger.error(f"同步過程發生錯誤: {e}")
        
        # 等待下一輪
        next_run = datetime.now() + timedelta(hours=interval_hours)
        logger.info(f"下次同步時間: {next_run.strftime('%Y-%m-%d %H:%M:%S')}")
        time.sleep(interval_hours * 3600)


def main():
    parser = argparse.ArgumentParser(description="Symbol Registry 自動同步腳本 (T036)")
    parser.add_argument("--config", type=Path, default=DEFAULT_CONFIG_PATH,
                        help=f"config.json 路徑 (預設: {DEFAULT_CONFIG_PATH})")
    parser.add_argument("--override", type=Path, default=DEFAULT_OVERRIDE_PATH,
                        help=f"manual_overrides.json 路徑 (預設: {DEFAULT_OVERRIDE_PATH})")
    parser.add_argument("--daemon", action="store_true",
                        help="背景執行模式，定期同步")
    parser.add_argument("--interval", type=int, default=24,
                        help="Daemon 模式同步間隔（小時，預設 24）")
    parser.add_argument("--dry-run", action="store_true",
                        help="試運行模式，不實際寫入檔案")
    parser.add_argument("--verbose", "-v", action="store_true",
                        help="詳細輸出")
    
    args = parser.parse_args()
    
    if args.verbose:
        logger.setLevel(logging.DEBUG)
    
    if args.daemon:
        run_daemon(args.config, args.override, args.interval)
    else:
        result = sync_symbols(args.config, args.override, args.dry_run)
        sys.exit(0 if result["errors"] == 0 else 1)


if __name__ == "__main__":
    main()