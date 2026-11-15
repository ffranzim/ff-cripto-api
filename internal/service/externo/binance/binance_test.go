package binance

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

func binanceTeste(srv *httptest.Server) httpjsontest.BitcoinFetcher {
	p := NewBinance(srv.Client())
	p.url, p.agora = srv.URL+"/api/v3/ticker/price", func() time.Time { return httpjsontest.AgoraTeste }
	return p
}

func TestBinance_Fetch(t *testing.T) {
	ok := `[{"symbol":"BTCUSDT","price":"67012.34000000"},{"symbol":"BTCBRL","price":"360123.00000000"}]`
	httpjsontest.RodarCasos(t, binanceTeste, domain.FonteBinance, []httpjsontest.Caso{
		{Name: "sucesso", Status: 200, Corpo: ok, WantUSD: "67012.34000000", WantBRL: "360123.00000000"},
		{Name: "ordem invertida", Status: 200, Corpo: `[{"symbol":"BTCBRL","price":"2"},{"symbol":"BTCUSDT","price":"1"}]`, WantUSD: "1", WantBRL: "2"},
		{Name: "JSON inválido", Status: 200, Corpo: `[{"symbol":`, WantErr: httpjson.ErrJSONInvalido},
		{Name: "objeto em vez de lista", Status: 200, Corpo: `{"code":-1121,"msg":"Invalid symbol."}`, WantErr: httpjson.ErrJSONInvalido},
		{Name: "sem BTCBRL", Status: 200, Corpo: `[{"symbol":"BTCUSDT","price":"1"}]`, WantErr: httpjson.ErrCampoAusente},
		{Name: "lista vazia", Status: 200, Corpo: `[]`, WantErr: httpjson.ErrCampoAusente},
		{Name: "preço não numérico", Status: 200, Corpo: `[{"symbol":"BTCUSDT","price":"abc"},{"symbol":"BTCBRL","price":"2"}]`, WantErr: httpjson.ErrPrecoInvalido},
		{Name: "preço zero", Status: 200, Corpo: `[{"symbol":"BTCUSDT","price":"0.00000000"},{"symbol":"BTCBRL","price":"2"}]`, WantErr: httpjson.ErrPrecoInvalido},
		{Name: "HTTP 451 (região bloqueada)", Status: 451, Corpo: `{"code":0}`, WantErr: httpjson.ErrHTTP},
	})
}

func TestBinance_SymbolsComURLEncode(t *testing.T) {
	var req *http.Request
	srv := httpjsontest.Servidor(t, 200, `[{"symbol":"BTCUSDT","price":"1"},{"symbol":"BTCBRL","price":"2"}]`, &req)
	if _, err := binanceTeste(srv).Fetch(context.Background()); err != nil {
		t.Fatal(err)
	}
	if want := "symbols=%5B%22BTCUSDT%22%2C%22BTCBRL%22%5D"; req.URL.RawQuery != want {
		t.Errorf("query = %q, queria %q", req.URL.RawQuery, want)
	}
	if got := req.URL.Query().Get("symbols"); got != `["BTCUSDT","BTCBRL"]` {
		t.Errorf("symbols decodificado = %q", got)
	}
}
