package coingecko

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/franzim/ff-cripto-api/internal/service/externo/httpjson"
	"github.com/franzim/ff-cripto-api/internal/service/externo/httpjson/httpjsontest"
)

func historicoTeste(srv *httptest.Server) *CoinGeckoHistorico {
	p := NewCoinGeckoHistorico("chave-teste", srv.Client())
	p.url = srv.URL + "/api/v3/coins/bitcoin/market_chart"
	return p
}

func TestCoinGeckoHistoricoSucesso(t *testing.T) {
	var req *http.Request
	srv := httpjsontest.Servidor(t, 200, `{
		"prices":        [[1762588800000, 67012.34], [1762592400000, 6.7e4], [1762592400000, 67100.5]],
		"market_caps":   [[1762588800000, 1325678901234.56], [1762592400000, 0]],
		"total_volumes": [[1762588800000, null], [1762592400000, 31234567890.12]]
	}`, &req)

	pontos, err := historicoTeste(srv).FetchHistory(context.Background(), "brl", 30)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	if got := req.URL.Query().Get("vs_currency"); got != "brl" {
		t.Errorf("vs_currency = %q", got)
	}
	if got := req.URL.Query().Get("days"); got != "30" {
		t.Errorf("days = %q", got)
	}
	if got := req.Header.Get("x-cg-demo-api-key"); got != "chave-teste" {
		t.Errorf("header da chave = %q", got)
	}

	// O instante repetido fica com o último valor.
	if len(pontos) != 2 {
		t.Fatalf("len = %d, queria 2: %+v", len(pontos), pontos)
	}
	p0, p1 := pontos[0], pontos[1]
	if !p0.At.Equal(time.Date(2025, 11, 8, 8, 0, 0, 0, time.UTC)) || p0.At.Location() != time.UTC {
		t.Errorf("At = %v", p0.At)
	}
	if p0.Price != "67012.34" || p0.MarketCap == nil || *p0.MarketCap != "1325678901234.56" || p0.TotalVolume != nil {
		t.Errorf("ponto 0 = %+v", p0)
	}
	if p1.Price != "67100.5" || p1.MarketCap == nil || *p1.MarketCap != "0" || p1.TotalVolume == nil || *p1.TotalVolume != "31234567890.12" {
		t.Errorf("ponto 1 = %+v", p1)
	}
}

func TestCoinGeckoHistoricoErros(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		corpo   string
		wantErr error
	}{
		{"429", 429, `{"status":{"error_code":429}}`, httpjson.ErrHTTP},
		{"sem prices", 200, `{"prices": []}`, httpjson.ErrCampoAusente},
		{"preço zero", 200, `{"prices": [[1762588800000, 0]]}`, httpjson.ErrPrecoInvalido},
		{"preço null", 200, `{"prices": [[1762588800000, null]]}`, httpjson.ErrCampoAusente},
		{"market cap negativo", 200, `{"prices": [[1762588800000, 1]], "market_caps": [[1762588800000, -1]]}`, httpjson.ErrPrecoInvalido},
		{"par incompleto", 200, `{"prices": [[1762588800000]]}`, httpjson.ErrJSONInvalido},
		{"fora de ordem", 200, `{"prices": [[1762592400000, 1], [1762588800000, 2]]}`, httpjson.ErrJSONInvalido},
		{"não é JSON", 200, `<html>`, httpjson.ErrJSONInvalido},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httpjsontest.Servidor(t, tt.status, tt.corpo, nil)
			_, err := historicoTeste(srv).FetchHistory(context.Background(), "usd", 30)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("erro = %v, queria %v", err, tt.wantErr)
			}
		})
	}
}
