package interno

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/franzim/ff-cripto-api/internal/domain"
	"github.com/franzim/ff-cripto-api/internal/service/externo/httpjson"
)

// fakeProvider devolve q ou err; com lento = true, espera o ctx acabar (simula timeout).
type fakeProvider struct {
	nome     string
	q        domain.BitcoinQuote
	err      error
	lento    bool
	chamadas int
}

func (f *fakeProvider) Name() string { return f.nome }

func (f *fakeProvider) Fetch(ctx context.Context) (domain.BitcoinQuote, error) {
	f.chamadas++
	if f.lento {
		<-ctx.Done()
		return domain.BitcoinQuote{}, ctx.Err()
	}
	return f.q, f.err
}

func ok(nome string) *fakeProvider {
	return &fakeProvider{nome: nome, q: domain.BitcoinQuote{USD: "1", BRL: "5", Source: nome, FetchedAt: agoraBTC}}
}

func falha(nome string, err error) *fakeProvider { return &fakeProvider{nome: nome, err: err} }

func novoFallback(logs *bytes.Buffer, providers ...NamedBitcoinQuoteProvider) *FallbackBitcoinQuoteProvider {
	log := slog.New(slog.NewTextHandler(logs, nil))
	return NewFallbackBitcoinQuoteProvider(log, 50*time.Millisecond, providers...)
}

func TestFallback_PrimeiroOK(t *testing.T) {
	a, b, c := ok("coingecko"), ok("binance"), ok("awesomeapi")
	var logs bytes.Buffer

	q, err := novoFallback(&logs, a, b, c).Fetch(context.Background())

	if err != nil || q.Source != "coingecko" {
		t.Fatalf("q = %+v, err = %v", q, err)
	}
	if b.chamadas != 0 || c.chamadas != 0 {
		t.Errorf("chamou os fallbacks sem necessidade (binance=%d, awesomeapi=%d)", b.chamadas, c.chamadas)
	}
	if logs.Len() != 0 {
		t.Errorf("logou sem falha: %s", logs.String())
	}
}

func TestFallback_PrimeiroFalhaSegundoOK(t *testing.T) {
	a, b, c := falha("coingecko", httpjson.ErrHTTP), ok("binance"), ok("awesomeapi")
	var logs bytes.Buffer

	q, err := novoFallback(&logs, a, b, c).Fetch(context.Background())

	if err != nil || q.Source != "binance" {
		t.Fatalf("q = %+v, err = %v", q, err)
	}
	if c.chamadas != 0 {
		t.Error("chamou o 3º provedor sem necessidade")
	}
	if !strings.Contains(logs.String(), "provedor=coingecko") {
		t.Errorf("falha do coingecko não foi logada: %s", logs.String())
	}
}

func TestFallback_PrimeiroESegundoFalhamTerceiroOK(t *testing.T) {
	a := &fakeProvider{nome: "coingecko", lento: true} // timeout
	b := falha("binance", httpjson.ErrJSONInvalido)
	c := ok("awesomeapi")
	var logs bytes.Buffer

	q, err := novoFallback(&logs, a, b, c).Fetch(context.Background())

	if err != nil || q.Source != "awesomeapi" {
		t.Fatalf("q = %+v, err = %v", q, err)
	}
	for _, want := range []string{"provedor=coingecko", "timeout", "provedor=binance", "JSON inválido"} {
		if !strings.Contains(logs.String(), want) {
			t.Errorf("log não contém %q: %s", want, logs.String())
		}
	}
}

func TestFallback_TodosFalham(t *testing.T) {
	a := &fakeProvider{nome: "coingecko", lento: true}
	b := falha("binance", httpjson.ErrCampoAusente)
	c := falha("awesomeapi", httpjson.ErrPrecoInvalido)
	var logs bytes.Buffer

	_, err := novoFallback(&logs, a, b, c).Fetch(context.Background())

	if !errors.Is(err, ErrTodosFalharam) || !errors.Is(err, domain.ErrIndisponivel) {
		t.Fatalf("erro = %v, queria ErrTodosFalharam", err)
	}
	// O erro agregado mantém o motivo de cada provedor.
	for _, motivo := range []error{context.DeadlineExceeded, httpjson.ErrCampoAusente, httpjson.ErrPrecoInvalido} {
		if !errors.Is(err, motivo) {
			t.Errorf("erro agregado não contém %v: %v", motivo, err)
		}
	}
	for _, nome := range []string{"coingecko", "binance", "awesomeapi"} {
		if !strings.Contains(err.Error(), nome+":") {
			t.Errorf("erro agregado não cita %s: %v", nome, err)
		}
		if !strings.Contains(logs.String(), "provedor="+nome) {
			t.Errorf("falha de %s não foi logada", nome)
		}
	}
}

func TestFallback_OrdemVemDaInjecao(t *testing.T) {
	a, b := ok("awesomeapi"), ok("coingecko")
	q, err := novoFallback(&bytes.Buffer{}, a, b).Fetch(context.Background())
	if err != nil || q.Source != "awesomeapi" {
		t.Fatalf("q = %+v, err = %v; queria o primeiro da lista injetada", q, err)
	}
}

func TestFallback_ContextoCanceladoNaoTentaOsProximos(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	b := ok("binance")

	_, err := novoFallback(&bytes.Buffer{}, b).Fetch(ctx)

	if !errors.Is(err, context.Canceled) || b.chamadas != 0 {
		t.Fatalf("erro = %v, chamadas = %d", err, b.chamadas)
	}
}
