package interno

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/franzim/ff-cripto-api/internal/domain"
)

var agoraBTC = time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)

const ttlTeste = 5 * time.Minute

var errProvedores = fmt.Errorf("%w: todos os provedores falharam", domain.ErrIndisponivel)

// fakeBTCProvider conta as chamadas; com liberar != nil, espera o canal antes de responder.
type fakeBTCProvider struct {
	q        domain.BitcoinQuote
	err      error
	liberar  chan struct{}
	chamadas atomic.Int32
}

func (f *fakeBTCProvider) Fetch(context.Context) (domain.BitcoinQuote, error) {
	f.chamadas.Add(1)
	if f.liberar != nil {
		<-f.liberar
	}
	return f.q, f.err
}

// fakeBTCRepo guarda as cotações em memória (seguro para goroutines).
type fakeBTCRepo struct {
	mu     sync.Mutex
	salvas []domain.BitcoinQuote
	errGet error
}

func (r *fakeBTCRepo) GetLast(context.Context) (domain.BitcoinQuote, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.errGet != nil {
		return domain.BitcoinQuote{}, r.errGet
	}
	if len(r.salvas) == 0 {
		return domain.BitcoinQuote{}, domain.ErrNaoEncontrado
	}
	return r.salvas[len(r.salvas)-1], nil
}

func (r *fakeBTCRepo) Save(_ context.Context, q domain.BitcoinQuote) (domain.BitcoinQuote, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	q.ID = int64(len(r.salvas) + 1)
	r.salvas = append(r.salvas, q)
	return q, nil
}

func (r *fakeBTCRepo) total() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.salvas)
}

func cotacaoEm(fonte string, em time.Time) domain.BitcoinQuote {
	return domain.BitcoinQuote{USD: "67000.5", BRL: "360000.25", Source: fonte, FetchedAt: em}
}

func novoBTCService(p *fakeBTCProvider, repo *fakeBTCRepo) *BitcoinQuoteService {
	s := NewBitcoinQuoteService(p, repo, ttlTeste)
	s.agora = func() time.Time { return agoraBTC } // relógio fixo
	return s
}

func TestBitcoinQuoteService_CacheValidoNaoChamaProvedor(t *testing.T) {
	salva := cotacaoEm("coingecko", agoraBTC.Add(-ttlTeste+time.Second)) // 1s antes de vencer
	repo := &fakeBTCRepo{salvas: []domain.BitcoinQuote{salva}}
	p := &fakeBTCProvider{q: cotacaoEm("binance", agoraBTC)}

	res, err := novoBTCService(p, repo).GetQuote(context.Background(), false)

	if err != nil {
		t.Fatal(err)
	}
	if res.Quote != salva || res.Stale {
		t.Errorf("res = %+v, queria a salva sem stale", res)
	}
	if n := p.chamadas.Load(); n != 0 {
		t.Errorf("provedor chamado %d vezes, queria 0", n)
	}
}

func TestBitcoinQuoteService_CacheVencidoChamaESalva(t *testing.T) {
	// Exatamente no TTL já conta como vencida (now - fetchedAt < TTL é o critério de válida).
	repo := &fakeBTCRepo{salvas: []domain.BitcoinQuote{cotacaoEm("coingecko", agoraBTC.Add(-ttlTeste))}}
	nova := cotacaoEm("binance", agoraBTC)
	p := &fakeBTCProvider{q: nova}

	res, err := novoBTCService(p, repo).GetQuote(context.Background(), false)

	if err != nil {
		t.Fatal(err)
	}
	if res.Stale || res.Quote.Source != "binance" || res.Quote.ID != 2 {
		t.Errorf("res = %+v, queria a nova cotação salva (id 2)", res)
	}
	if p.chamadas.Load() != 1 || repo.total() != 2 {
		t.Errorf("chamadas = %d, salvas = %d; queria 1 e 2 (histórico mantido)", p.chamadas.Load(), repo.total())
	}
}

func TestBitcoinQuoteService_SemCacheChamaESalva(t *testing.T) {
	repo := &fakeBTCRepo{}
	p := &fakeBTCProvider{q: cotacaoEm("coingecko", agoraBTC)}

	res, err := novoBTCService(p, repo).GetQuote(context.Background(), false)

	if err != nil || res.Stale || res.Quote.ID != 1 || repo.total() != 1 {
		t.Fatalf("res = %+v, err = %v, salvas = %d", res, err, repo.total())
	}
}

func TestBitcoinQuoteService_FalhaGeralComCacheVencidoRetornaStale(t *testing.T) {
	vencida := cotacaoEm("awesomeapi", agoraBTC.Add(-time.Hour))
	repo := &fakeBTCRepo{salvas: []domain.BitcoinQuote{vencida}}
	p := &fakeBTCProvider{err: errProvedores}

	res, err := novoBTCService(p, repo).GetQuote(context.Background(), false)

	if err != nil {
		t.Fatal(err)
	}
	if !res.Stale || res.Quote != vencida {
		t.Errorf("res = %+v, queria a vencida com stale", res)
	}
	if repo.total() != 1 {
		t.Error("não deveria salvar nada")
	}
}

func TestBitcoinQuoteService_FalhaGeralSemCachePropagaErro(t *testing.T) {
	p := &fakeBTCProvider{err: errProvedores}

	_, err := novoBTCService(p, &fakeBTCRepo{}).GetQuote(context.Background(), false)

	if !errors.Is(err, errProvedores) || !errors.Is(err, domain.ErrIndisponivel) {
		t.Fatalf("erro = %v, queria o erro dos provedores", err)
	}
}

func TestBitcoinQuoteService_ForceRefreshIgnoraCache(t *testing.T) {
	repo := &fakeBTCRepo{salvas: []domain.BitcoinQuote{cotacaoEm("coingecko", agoraBTC)}} // novinha
	p := &fakeBTCProvider{q: cotacaoEm("binance", agoraBTC)}

	res, err := novoBTCService(p, repo).GetQuote(context.Background(), true)

	if err != nil {
		t.Fatal(err)
	}
	if p.chamadas.Load() != 1 || res.Quote.Source != "binance" || repo.total() != 2 {
		t.Errorf("chamadas = %d, res = %+v, salvas = %d", p.chamadas.Load(), res, repo.total())
	}
}

func TestBitcoinQuoteService_ForceRefreshComFalhaRetornaStale(t *testing.T) {
	salva := cotacaoEm("coingecko", agoraBTC)
	repo := &fakeBTCRepo{salvas: []domain.BitcoinQuote{salva}}
	p := &fakeBTCProvider{err: errProvedores}

	res, err := novoBTCService(p, repo).GetQuote(context.Background(), true)

	if err != nil || !res.Stale || res.Quote != salva {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
}

func TestBitcoinQuoteService_ErroDoBancoNaLeituraEPropagado(t *testing.T) {
	errBanco := errors.New("banco fora")
	p := &fakeBTCProvider{q: cotacaoEm("coingecko", agoraBTC)}

	_, err := novoBTCService(p, &fakeBTCRepo{errGet: errBanco}).GetQuote(context.Background(), false)

	if !errors.Is(err, errBanco) || p.chamadas.Load() != 0 {
		t.Fatalf("erro = %v, chamadas = %d", err, p.chamadas.Load())
	}
}

func TestBitcoinQuoteService_ChamadasConcorrentesCompartilhamABusca(t *testing.T) {
	repo := &fakeBTCRepo{}
	p := &fakeBTCProvider{q: cotacaoEm("coingecko", agoraBTC), liberar: make(chan struct{})}
	svc := novoBTCService(p, repo)

	const n = 20
	var wg sync.WaitGroup
	resultados := make([]domain.BitcoinQuoteResult, n)
	erros := make([]error, n)
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resultados[i], erros[i] = svc.GetQuote(context.Background(), false)
		}()
	}

	// Espera a 1ª busca começar e dá tempo das outras goroutines entrarem na fila.
	for p.chamadas.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)
	close(p.liberar)
	wg.Wait()

	if c := p.chamadas.Load(); c != 1 {
		t.Errorf("provedor chamado %d vezes, queria 1", c)
	}
	if repo.total() != 1 {
		t.Errorf("salvou %d vezes, queria 1", repo.total())
	}
	for i := range n {
		if erros[i] != nil || resultados[i].Quote.ID != 1 {
			t.Fatalf("goroutine %d: res = %+v, err = %v", i, resultados[i], erros[i])
		}
	}
}

func TestBitcoinQuoteService_QuemEsperaRespeitaOProprioContexto(t *testing.T) {
	p := &fakeBTCProvider{q: cotacaoEm("coingecko", agoraBTC), liberar: make(chan struct{})}
	defer close(p.liberar)
	svc := novoBTCService(p, &fakeBTCRepo{})

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := svc.GetQuote(ctx, false)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("erro = %v, queria DeadlineExceeded", err)
	}
}

func TestNewBitcoinQuoteService_TTLPadrao(t *testing.T) {
	if s := NewBitcoinQuoteService(nil, nil, 0); s.ttl != TTLPadraoCotacaoBitcoin {
		t.Errorf("ttl = %v, queria %v", s.ttl, TTLPadraoCotacaoBitcoin)
	}
}
