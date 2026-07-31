package provider

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"tw-quant-mcp/pkg/model"
)

// fakeSource 實作 SourceContract（§2.2）之最小範例。
type fakeSource struct {
	id  string
	raw *RawResponse
}

var _ SourceContract = (*fakeSource)(nil)

func (f *fakeSource) ID() string { return f.id }

func (f *fakeSource) Fetch(_ context.Context, _ RawRequest) (*RawResponse, error) {
	if f.raw == nil {
		return nil, errors.New("no data")
	}
	return f.raw, nil
}

func (f *fakeSource) Validate(raw *RawResponse) error {
	if len(raw.Body) == 0 {
		return errors.New("empty body")
	}
	return nil
}

func (f *fakeSource) Normalize(raw *RawResponse) ([]byte, error) {
	if len(raw.Body) == 0 {
		return nil, errors.New("empty body")
	}
	return raw.Body, nil
}

func TestSourceContractSmoke(t *testing.T) {
	src := &fakeSource{id: model.SourceTWSEWeb}

	var contract SourceContract = src
	if contract.ID() != "TWSE_WEB" {
		t.Errorf("ID = %q", contract.ID())
	}
	if _, err := contract.Fetch(context.Background(), RawRequest{}); err == nil {
		t.Error("無資料時 Fetch 應失敗")
	}

	src.raw = &RawResponse{Body: []byte(`{"ok":true}`)}
	raw, err := contract.Fetch(context.Background(), RawRequest{})
	if err != nil {
		t.Fatalf("Fetch 失敗: %v", err)
	}
	if err := contract.Validate(raw); err != nil {
		t.Fatalf("Validate 失敗: %v", err)
	}
	out, err := contract.Normalize(raw)
	if err != nil {
		t.Fatalf("Normalize 失敗: %v", err)
	}
	if string(out) != `{"ok":true}` {
		t.Errorf("Normalize 輸出 = %q", out)
	}
}

// TestSourceContractWithBaseClient 驗證契約之 Fetch 可建在 BaseClient 之上。
func TestSourceContractWithBaseClient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":"raw"}`))
	}))
	defer srv.Close()

	c := NewBaseClient("test.host", WithRateInterval(0))
	src := &fakeSource{id: model.SourceTWSEWeb}
	src.raw = nil

	raw, err := c.Do(context.Background(), RawRequest{URL: srv.URL})
	if err != nil {
		t.Fatalf("BaseClient.Do 失敗: %v", err)
	}
	src.raw = raw
	out, err := src.Normalize(raw)
	if err != nil {
		t.Fatalf("Normalize 失敗: %v", err)
	}
	if string(out) != `{"data":"raw"}` {
		t.Errorf("Normalize 輸出 = %q", out)
	}
}
