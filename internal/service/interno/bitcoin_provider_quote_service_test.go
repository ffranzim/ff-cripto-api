package interno

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/franzim/ff-cripto-api/internal/domain"
)

func novoProviderService(providers map[string]BitcoinQuoteProvider, agora *time.Time) *BitcoinProviderQuoteService {
	s := NewBitcoinProviderQuoteService(providers, ttlTeste, time.Second)
	s.agora = func() time.Time { return *agora }
	return s
}

func TestBitcoinProviderQuoteService_ChamaSoOProvedorPedido(t *testing.T) {
	agora := agoraBTC
	cg := &fakeBTCProvider{q: cotacaoEm(domain.FonteCoinGecko, agoraBTC)}
	bn := &fakeBTCProvider{q: cotacaoEm(domain.FonteBinance, agoraBTC)}
	s := novoProviderService(map[string]BitcoinQuoteProvider{domain.FonteCoinGecko: cg, domain.FonteBinance: bn}, &agora)

	q, err := s.GetQuoteFrom(context.Background(), domain.FonteBinance)
	if err != nil {
		t.Fatal(err)
	}
	if q.Source != domain.FonteBinance {
		t.Errorf("source = %q", q.Source)
	}
	if cg.chamadas.Load() != 0 || bn.chamadas.Load() != 1 {
		t.Errorf("chamadas coingecko=%d binance=%d", cg.chamadas.Load(), bn.chamadas.Load())
	}
}

func TestBitcoinProviderQuoteService_CacheEExpiracao(t *testing.T) {
	agora := agoraBTC
	p := &fakeBTCProvider{q: cotacaoEm(domain.FonteCoinGecko, agoraBTC)}
	s := novoProviderService(map[string]BitcoinQuoteProvider{domain.FonteCoinGecko: p}, &agora)

	for range 3 {
		if _, err := s.GetQuoteFrom(context.Background(), domain.FonteCoinGecko); err != nil {
			t.Fatal(err)
		}
	}
	if n := p.chamadas.Load(); n != 1 {
		t.Fatalf("dentro do TTL: chamadas = %d, queria 1", n)
	}

	agora = agoraBTC.Add(ttlTeste) // venceu
	if _, err := s.GetQuoteFrom(context.Background(), domain.FonteCoinGecko); err != nil {
		t.Fatal(err)
	}
	if n := p.chamadas.Load(); n != 2 {
		t.Fatalf("após o TTL: chamadas = %d, queria 2", n)
	}
}

func TestBitcoinProviderQuoteService_FonteDesconhecida(t *testing.T) {
	agora := agoraBTC
	s := novoProviderService(map[string]BitcoinQuoteProvider{}, &agora)
	_, err := s.GetQuoteFrom(context.Background(), "kraken")
	if !errors.Is(err, domain.ErrNaoEncontrado) {
		t.Fatalf("err = %v, queria ErrNaoEncontrado", err)
	}
}

func TestBitcoinProviderQuoteService_FalhaViraIndisponivelESemCache(t *testing.T) {
	agora := agoraBTC
	p := &fakeBTCProvider{err: errors.New("status 429")}
	s := novoProviderService(map[string]BitcoinQuoteProvider{domain.FonteAwesomeAPI: p}, &agora)

	for range 2 {
		_, err := s.GetQuoteFrom(context.Background(), domain.FonteAwesomeAPI)
		if !errors.Is(err, domain.ErrIndisponivel) {
			t.Fatalf("err = %v, queria ErrIndisponivel", err)
		}
	}
	if n := p.chamadas.Load(); n != 2 {
		t.Errorf("falha não deve ir para o cache: chamadas = %d, queria 2", n)
	}
}

func TestBitcoinProviderQuoteService_SimultaneasUmaBusca(t *testing.T) {
	agora := agoraBTC
	p := &fakeBTCProvider{q: cotacaoEm(domain.FonteBinance, agoraBTC), liberar: make(chan struct{})}
	s := novoProviderService(map[string]BitcoinQuoteProvider{domain.FonteBinance: p}, &agora)

	var wg sync.WaitGroup
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.GetQuoteFrom(context.Background(), domain.FonteBinance); err != nil {
				t.Error(err)
			}
		}()
	}
	// Espera a primeira busca começar antes de liberar.
	for p.chamadas.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	time.Sleep(20 * time.Millisecond) // dá tempo das outras entrarem no singleflight
	close(p.liberar)
	wg.Wait()

	if n := p.chamadas.Load(); n != 1 {
		t.Errorf("chamadas = %d, queria 1", n)
	}
}
