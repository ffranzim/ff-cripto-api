package awesomeapi

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

func moedasTeste(srv *httptest.Server) *AwesomeAPIMoedas {
	p := NewAwesomeAPIMoedas("chave-teste", srv.Client())
	p.url = srv.URL + "/json/last/USD-BRL,EUR-BRL,GBP-BRL"
	return p
}

const corpoMoedas = `{
 "USDBRL":{"code":"USD","codein":"BRL","name":"Dólar Americano/Real Brasileiro","high":"5.5275","low":"5.4898","varBid":"-0.0035","pctChange":"-0.063576","bid":"5.5085","ask":"5.5091","timestamp":"1794264234"},
 "EURBRL":{"code":"EUR","codein":"BRL","name":"Euro/Real Brasileiro","high":"6.1899","low":"6.1335","varBid":"0.0462","pctChange":"0.753239","bid":"6.1797","ask":"6.1817","timestamp":"1794271892"},
 "GBPBRL":{"code":"GBP","codein":"BRL","name":"Libra Esterlina/Real Brasileiro","high":"7.411","low":"7.34214","varBid":"5e-2","pctChange":"0.708414","bid":"7.40203","ask":"7.40606","timestamp":"1794272170"}
}`

func TestAwesomeAPIMoedasSucesso(t *testing.T) {
	var req *http.Request
	srv := httpjsontest.Servidor(t, 200, corpoMoedas, &req)
	qs, err := moedasTeste(srv).FetchCurrencies(context.Background())
	if err != nil {
		t.Fatalf("erro: %v", err)
	}
	if got := req.Header.Get("x-api-key"); got != "chave-teste" {
		t.Errorf("x-api-key = %q", got)
	}
	if len(qs) != 3 || qs[0].Code != "USD" || qs[1].Code != "EUR" || qs[2].Code != "GBP" {
		t.Fatalf("ordem/qtde errada: %+v", qs)
	}
	usd := qs[0]
	if usd.CodeIn != "BRL" || usd.Bid != "5.5085" || usd.Ask != "5.5091" || usd.High != "5.5275" || usd.Low != "5.4898" ||
		usd.VarBid != "-0.0035" || usd.PctChange != "-0.063576" || !usd.QuotedAt.Equal(time.Unix(1794264234, 0)) {
		t.Errorf("USD = %+v", usd)
	}
	if qs[2].VarBid != "0.05000000" {
		t.Errorf("varBid em notação científica = %q", qs[2].VarBid)
	}
}

func TestAwesomeAPIMoedasErros(t *testing.T) {
	tests := []struct {
		name, corpo string
		status      int
		wantErr     error
	}{
		{"429", `{"status":429,"code":"QuotaExceeded"}`, 429, httpjson.ErrHTTP},
		{"falta GBPBRL", `{"USDBRL":{},"EURBRL":{}}`, 200, httpjson.ErrCampoAusente},
		{"bid zero", `{"USDBRL":{"code":"USD","codein":"BRL","bid":"0","ask":"1","high":"1","low":"1","varBid":"0","pctChange":"0","timestamp":"1"},"EURBRL":{},"GBPBRL":{}}`, 200, httpjson.ErrPrecoInvalido},
		{"varBid inválido", `{"USDBRL":{"code":"USD","codein":"BRL","bid":"1","ask":"1","high":"1","low":"1","varBid":"x","pctChange":"0","timestamp":"1"},"EURBRL":{},"GBPBRL":{}}`, 200, httpjson.ErrPrecoInvalido},
		{"timestamp inválido", `{"USDBRL":{"code":"USD","codein":"BRL","bid":"1","ask":"1","high":"1","low":"1","varBid":"0","pctChange":"0","timestamp":"ontem"},"EURBRL":{},"GBPBRL":{}}`, 200, httpjson.ErrJSONInvalido},
		{"não é JSON", `<html>`, 200, httpjson.ErrJSONInvalido},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := moedasTeste(httpjsontest.Servidor(t, tt.status, tt.corpo, nil)).FetchCurrencies(context.Background())
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("erro = %v, queria %v", err, tt.wantErr)
			}
		})
	}
}

func TestAwesomeAPIMoedasDiario(t *testing.T) {
	var req *http.Request
	srv := httpjsontest.Servidor(t, 200, `[
	 {"code":"USD","codein":"BRL","name":"Dólar Americano/Real Brasileiro","high":"5.5275","low":"5.4898","varBid":"0.0036","pctChange":"0.065393","bid":"5.5086","ask":"5.5091","timestamp":"1794264226"},
	 {"high":"5.505","low":"5.503","varBid":"0.002","pctChange":"0.036348","bid":"5.505","ask":"5.5055","timestamp":"1794182265"},
	 {"high":"5.5235","low":"5.4835","varBid":"-0.0008","pctChange":"-0.014534","bid":"5.503","ask":"5.5035","timestamp":"1794002689"}
	]`, &req)
	p := moedasTeste(srv)
	p.urlDiario = srv.URL + "/json/daily/"

	pts, err := p.FetchDaily(context.Background(), "USD-BRL", 30)
	if err != nil {
		t.Fatalf("erro: %v", err)
	}
	if req.URL.Path != "/json/daily/USD-BRL/30" || req.Header.Get("x-api-key") != "chave-teste" {
		t.Errorf("request = %s, x-api-key %q", req.URL.Path, req.Header.Get("x-api-key"))
	}
	if len(pts) != 3 {
		t.Fatalf("len = %d", len(pts))
	}
	// Ordem cronológica e code/name do par em todos os pontos.
	if pts[0].Bid != "5.503" || pts[0].VarBid != "-0.0008" || pts[2].Bid != "5.5086" {
		t.Errorf("ordem errada: %+v", pts)
	}
	for _, q := range pts {
		if q.Code != "USD" || q.CodeIn != "BRL" || q.Name != "Dólar Americano/Real Brasileiro" {
			t.Errorf("ponto sem code/name: %+v", q)
		}
	}
	if !pts[0].QuotedAt.Before(pts[1].QuotedAt) {
		t.Errorf("não está em ordem cronológica")
	}
}

func TestAwesomeAPIMoedasDiarioErros(t *testing.T) {
	for _, c := range []struct {
		name, corpo string
		status      int
		wantErr     error
	}{
		{"vazio", `[]`, 200, httpjson.ErrCampoAusente},
		{"bid ausente", `[{"ask":"1","high":"1","low":"1","varBid":"0","pctChange":"0","timestamp":"1"}]`, 200, httpjson.ErrCampoAusente},
		{"404", `{"status":404,"code":"CoinNotExists"}`, 404, httpjson.ErrHTTP},
		{"objeto em vez de lista", `{"USDBRL":{}}`, 200, httpjson.ErrJSONInvalido},
	} {
		t.Run(c.name, func(t *testing.T) {
			srv := httpjsontest.Servidor(t, c.status, c.corpo, nil)
			p := moedasTeste(srv)
			p.urlDiario = srv.URL + "/json/daily/"
			if _, err := p.FetchDaily(context.Background(), "USD-BRL", 30); !errors.Is(err, c.wantErr) {
				t.Fatalf("erro = %v, queria %v", err, c.wantErr)
			}
		})
	}
}
