package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
)

// KeyString 依 §4.3 建構快取鍵：
//
//	cache_key := sha256(source_id | dataset | data_date | symbol | params_hash)[0:16]
//
// params 以鍵名排序序列化為 k=v 連綴，再取 sha256 前 16 字元作為 params_hash；
// params 為空時 params_hash 為空字串。回傳 16 字元小寫十六進位字串。
func KeyString(sourceID, dataset, dataDate, symbol string, params map[string]string) string {
	ph := ""
	if len(params) > 0 {
		keys := make([]string, 0, len(params))
		for k := range params {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var sb strings.Builder
		for i, k := range keys {
			if i > 0 {
				sb.WriteByte('&')
			}
			sb.WriteString(k)
			sb.WriteByte('=')
			sb.WriteString(params[k])
		}
		sum := sha256.Sum256([]byte(sb.String()))
		ph = hex.EncodeToString(sum[:8])
	}
	payload := strings.Join([]string{sourceID, dataset, dataDate, symbol, ph}, "|")
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:8])
}
