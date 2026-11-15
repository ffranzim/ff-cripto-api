package coingecko

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/franzim/ff-cripto-api/internal/domain"
	"github.com/franzim/ff-cripto-api/internal/service/externo/httpjson"
	"github.com/franzim/ff-cripto-api/internal/service/externo/httpjson/httpjsontest"
)

func coinGeckoTeste(srv *httptest.Server) httpjsontest.BitcoinFetcher {
	p := NewCoinGecko("chave-teste", srv.Client())
	p.url, p.agora = srv.URL+"/api/v3/simple/price?ids=bitcoin&vs_currencies=usd,brl", func() time.Time { return httpjsontest.AgoraTeste }
	return p
}

func TestCoinGecko_Fetch(t *testing.T) {
	httpjsontest.RodarCasos(t, coinGeckoTeste, domain.FonteCoinGecko, []httpjsontest.Caso{
		{Name: "sucesso", Status: 200, Corpo: `{"bitcoin":{"usd":67012.34,"brl":360123}}`, WantUSD: "67012.34", WantBRL: "360123"},
		{Name: "notação científica é normalizada", Status: 200, Corpo: `{"bitcoin":{"usd":6.7e4,"brl":3.6e5}}`, WantUSD: "67000.00000000", WantBRL: "360000.00000000"},
		{Name: "JSON inválido", Status: 200, Corpo: `{"bitcoin":`, WantErr: httpjson.ErrJSONInvalido},
		{Name: "sem objeto bitcoin", Status: 200, Corpo: `{}`, WantErr: httpjson.ErrCampoAusente},
		{Name: "sem brl", Status: 200, Corpo: `{"bitcoin":{"usd":67012.34}}`, WantErr: httpjson.ErrCampoAusente},
		{Name: "usd null", Status: 200, Corpo: `{"bitcoin":{"usd":null,"brl":1}}`, WantErr: httpjson.ErrCampoAusente},
		{Name: "preço zero", Status: 200, Corpo: `{"bitcoin":{"usd":0,"brl":1}}`, WantErr: httpjson.ErrPrecoInvalido},
		{Name: "preço negativo", Status: 200, Corpo: `{"bitcoin":{"usd":1,"brl":-5}}`, WantErr: httpjson.ErrPrecoInvalido},
		{Name: "HTTP 429", Status: 429, Corpo: `{"status":{"error_code":429}}`, WantErr: httpjson.ErrHTTP},
		{Name: "HTTP 500", Status: 500, Corpo: ``, WantErr: httpjson.ErrHTTP},
	})
}

func TestCoinGecko_EnviaChave(t *testing.T) {
	var req *http.Request
	srv := httpjsontest.Servidor(t, 200, `{"bitcoin":{"usd":1,"brl":2}}`, &req)
	if _, err := coinGeckoTeste(srv).Fetch(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := req.Header.Get("x-cg-demo-api-key"); got != "chave-teste" {
		t.Errorf("x-cg-demo-api-key = %q", got)
	}
	if got := req.URL.Query().Get("vs_currencies"); got != "usd,brl" {
		t.Errorf("vs_currencies = %q", got)
	}
}

func TestCoinGecko_SemChaveNaoEnviaHeader(t *testing.T) {
	var req *http.Request
	srv := httpjsontest.Servidor(t, 200, `{"bitcoin":{"usd":1,"brl":2}}`, &req)
	p := coinGeckoTeste(srv).(*CoinGecko)
	p.apiKey = ""
	if _, err := p.Fetch(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, ok := req.Header["X-Cg-Demo-Api-Key"]; ok {
		t.Error("header enviado sem chave configurada")
	}
}
