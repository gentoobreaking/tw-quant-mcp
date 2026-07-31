// Package provider 實作官方資料來源 Adapter 層（規格書 §2、§6）。
// 本包為 SourceContract（§2.2）、Resilient HTTP Client、每主機 Rate Limiter、
// Jitter、403/429 指數退避與 Circuit Breaker（§4.4）之唯一實作所在。
package provider

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"

	"tw-quant-mcp/pkg/model"
)

// RawRequest 為對官方來源之原始請求描述。
type RawRequest struct {
	Method  string      // 預設 GET
	URL     string      // 完整 URL（含 query 參數）
	Headers http.Header // 額外 header（如 content-type）；User-Agent 由 BaseClient 注入
}

// RawResponse 為官方來源之原始回應。僅內部暫存（§3.1 raw capture），
// 絕不回傳給 Client；Normalize 後才可進入後續管線。
type RawResponse struct {
	StatusCode int
	Header     http.Header
	Body       []byte // 原始 body（原文）
	BodyHash   string // sha256(body) hex（§3.1：含原文 hash）
	FetchedAt  model.TaipeiTime
	SourceURL  string
}

// SourceContract 是每個來源 Adapter 需實作之契約（§2.2）。
type SourceContract interface {
	ID() string // 對應 §2 登錄表之 ID（如 "TWSE_WEB"）
	Fetch(ctx context.Context, req RawRequest) (*RawResponse, error)
	Validate(raw *RawResponse) error            // schema 檢查（欄位存在性、數值範圍、日期一致性）
	Normalize(raw *RawResponse) ([]byte, error) // 轉為 Normalized Model（JSON）
}

// newRawResponse 由 http.Response 建立 RawResponse，並計算 raw body 之 sha256。
func newRawResponse(resp *http.Response, sourceURL string) (*RawResponse, error) {
	var body []byte
	if resp.Body != nil {
		defer resp.Body.Close()
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		body = b
	}
	h := sha256.Sum256(body)
	return &RawResponse{
		StatusCode: resp.StatusCode,
		Header:     resp.Header.Clone(),
		Body:       body,
		BodyHash:   hex.EncodeToString(h[:]),
		FetchedAt:  model.Now(),
		SourceURL:  sourceURL,
	}, nil
}
