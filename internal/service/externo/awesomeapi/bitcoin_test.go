package awesomeapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/franzim/ff-cripto-api/internal/domain"
	"github.com/franzim/ff-cripto-api/internal/service/externo/httpjson"
	"github.com/franzim/ff-cripto-api/internal/service/externo/httpjson/httpjsontest"
)

func awesomeTeste(srv *httptest.Server) httpjsontest.BitcoinFetcher {
	p := NewAwesomeAPI("chave-teste", srv.Client())
	p.url, p.agora = srv.URL+"/json/last/BTC-USD,BTC-BRL", func() time.Time { return httpjsontest.AgoraTeste }
	return p
}

func TestAwesomeAPI_Fetch(t *testing.T) {
	httpjsontest.RodarCasos(t, awesomeTeste, domain.FonteAwesomeAPI, []httpjsontest.Caso{
		{Name: "sucesso", Status: 200, Corpo: `{"BTCUSD":{"bid":"67012.3"},"BTCBRL":{"bid":"360123"}}`, WantUSD: "67012.3", WantBRL: "360123"},
		{Name: "JSON inválido", Status: 200, Corpo: `<html>`, WantErr: httpjson.ErrJSONInvalido},
		{Name: "sem BTCBRL", Status: 200, Corpo: `{"BTCUSD":{"bid":"1"}}`, WantErr: httpjson.ErrCampoAusente},
		{Name: "sem bid", Status: 200, Corpo: `{"BTCUSD":{"ask":"1"},"BTCBRL":{"bid":"2"}}`, WantErr: httpjson.ErrCampoAusente},
		{Name: "preço zero", Status: 200, Corpo: `{"BTCUSD":{"bid":"1"},"BTCBRL":{"bid":"0"}}`, WantErr: httpjson.ErrPrecoInvalido},
		{Name: "HTTP 404", Status: 404, Corpo: `{"status":404,"code":"CoinNotExists"}`, WantErr: httpjson.ErrHTTP},
	})
}

func TestProvedor_TimeoutDoContexto(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done() // nunca responde
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := awesomeTeste(srv).Fetch(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("erro = %v, queria DeadlineExceeded", err)
	}
}

func TestAwesomeAPI_EnviaChave(t *testing.T) {
	var req *http.Request
	srv := httpjsontest.Servidor(t, 200, `{"BTCUSD":{"bid":"1"},"BTCBRL":{"bid":"2"}}`, &req)
	if _, err := awesomeTeste(srv).Fetch(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := req.Header.Get("x-api-key"); got != "chave-teste" {
		t.Errorf("x-api-key = %q", got)
	}
}

func TestAwesomeAPI_SemChaveNaoEnviaHeader(t *testing.T) {
	var req *http.Request
	srv := httpjsontest.Servidor(t, 200, `{"BTCUSD":{"bid":"1"},"BTCBRL":{"bid":"2"}}`, &req)
	p := awesomeTeste(srv).(*AwesomeAPI)
	p.apiKey = ""
	if _, err := p.Fetch(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, ok := req.Header["X-Api-Key"]; ok {
		t.Error("header enviado sem chave configurada")
	}
}
