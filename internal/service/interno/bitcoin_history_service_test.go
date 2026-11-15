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

// fakeHistProvider: pontos = janela de hora em hora; diarios = janela diária.
type fakeHistProvider struct {
	pontos   []domain.BitcoinHistoryPoint
	diarios  []domain.BitcoinHistoryPoint
	err      error
	liberar  chan struct{}
	chamadas atomic.Int32
}

func (f *fakeHistProvider) FetchHistory(_ context.Context, _ string, dias int) ([]domain.BitcoinHistoryPoint, error) {
	f.chamadas.Add(1)
	if f.liberar != nil {
		<-f.liberar
	}
	switch dias {
	case DiasHistoricoBitcoinPorHora:
		return f.pontos, f.err
	case DiasHistoricoBitcoin:
		return f.diarios, f.err
	}
	return nil, fmt.Errorf("dias = %d", dias)
}

// fakeHistRepo guarda os pontos por moeda; sincronizado vem do relógio do teste.
type fakeHistRepo struct {
	mu           sync.Mutex
	pontos       map[string][]domain.BitcoinHistoryPoint
	sincronizado map[string]time.Time
	agora        func() time.Time
	errList      error
}

func novoFakeHistRepo(agora func() time.Time) *fakeHistRepo {
	return &fakeHistRepo{pontos: map[string][]domain.BitcoinHistoryPoint{}, sincronizado: map[string]time.Time{}, agora: agora}
}

func (r *fakeHistRepo) Replace(_ context.Context, moeda string, p []domain.BitcoinHistoryPoint) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pontos[moeda] = p
	r.sincronizado[moeda] = r.agora()
	return nil
}

func (r *fakeHistRepo) List(_ context.Context, moeda string, desde time.Time) ([]domain.BitcoinHistoryPoint, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.errList != nil {
		return nil, r.errList
	}
	var out []domain.BitcoinHistoryPoint
	for _, p := range r.pontos[moeda] {
		if !p.At.Before(desde) {
			out = append(out, p)
		}
	}
	return out, nil
}

func (r *fakeHistRepo) LastSync(_ context.Context, moeda string) (time.Time, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.sincronizado[moeda]
	return t, ok, nil
}

// pontosHoraEmHora: um ponto por hora nos últimos 90 dias até agoraBTC.
func pontosHoraEmHora() []domain.BitcoinHistoryPoint {
	var p []domain.BitcoinHistoryPoint
	for t := agoraBTC.Add(-DiasHistoricoBitcoinPorHora * 24 * time.Hour); !t.After(agoraBTC); t = t.Add(time.Hour) {
		p = append(p, domain.BitcoinHistoryPoint{At: t, Price: "67000"})
	}
	return p
}

// pontosDiarios: como a CoinGecko com days > 90, um ponto às 00:00 UTC de cada
// dia dos últimos 360 dias e o último no instante atual.
func pontosDiarios() []domain.BitcoinHistoryPoint {
	var p []domain.BitcoinHistoryPoint
	inicio := agoraBTC.Truncate(24 * time.Hour).Add(-DiasHistoricoBitcoin * 24 * time.Hour)
	for t := inicio; t.Before(agoraBTC); t = t.Add(24 * time.Hour) {
		p = append(p, domain.BitcoinHistoryPoint{At: t, Price: "50000"})
	}
	return append(p, domain.BitcoinHistoryPoint{At: agoraBTC, Price: "67000"})
}

func novoFakeHistProvider() *fakeHistProvider {
	return &fakeHistProvider{pontos: pontosHoraEmHora(), diarios: pontosDiarios()}
}

func novoServicoHistorico(prov *fakeHistProvider) (*BitcoinHistoryService, *fakeHistRepo, *time.Time) {
	agora := agoraBTC
	relogio := func() time.Time { return agora }
	repo := novoFakeHistRepo(relogio)
	s := NewBitcoinHistoryService(prov, repo, time.Hour)
	s.agora = relogio
	return s, repo, &agora
}

func TestHistoricoSincronizaQuandoVazio(t *testing.T) {
	prov := novoFakeHistProvider()
	s, _, _ := novoServicoHistorico(prov)

	h, err := s.GetHistory(context.Background(), domain.MoedaBRL, 0)
	if err != nil {
		t.Fatalf("erro: %v", err)
	}
	if h.Currency != "brl" || h.Days != 30 || h.Interval != domain.IntervaloHora || h.Stale || !h.UpdatedAt.Equal(agoraBTC) {
		t.Errorf("histórico = %+v", h)
	}
	if len(h.Points) != 30*24+1 {
		t.Errorf("pontos = %d", len(h.Points))
	}
	if prov.chamadas.Load() != 2 { // diária + de hora em hora
		t.Errorf("chamadas = %d", prov.chamadas.Load())
	}
}

func TestHistoricoUsaCacheDentroDoTTL(t *testing.T) {
	prov := novoFakeHistProvider()
	s, _, agora := novoServicoHistorico(prov)
	ctx := context.Background()

	if _, err := s.GetHistory(ctx, domain.MoedaUSD, 0); err != nil {
		t.Fatal(err)
	}
	*agora = agoraBTC.Add(59 * time.Minute)
	h, err := s.GetHistory(ctx, domain.MoedaUSD, 7)
	if err != nil {
		t.Fatal(err)
	}
	if prov.chamadas.Load() != 2 {
		t.Errorf("chamadas = %d, queria 2 (cache)", prov.chamadas.Load())
	}
	// 7 dias contados de agora (59 min depois): o primeiro ponto da janela cai fora.
	if len(h.Points) != 7*24 || h.Days != 7 {
		t.Errorf("pontos = %d, days = %d", len(h.Points), h.Days)
	}

	// Outra moeda tem sincronização própria.
	if _, err := s.GetHistory(ctx, domain.MoedaBRL, 0); err != nil {
		t.Fatal(err)
	}
	if prov.chamadas.Load() != 4 {
		t.Errorf("chamadas = %d, queria 4", prov.chamadas.Load())
	}

	*agora = agoraBTC.Add(time.Hour)
	if _, err := s.GetHistory(ctx, domain.MoedaUSD, 0); err != nil {
		t.Fatal(err)
	}
	if prov.chamadas.Load() != 6 {
		t.Errorf("chamadas = %d, queria 6 (TTL vencido)", prov.chamadas.Load())
	}
}

func TestHistoricoFalhaComDadosSalvosViraStale(t *testing.T) {
	prov := novoFakeHistProvider()
	s, _, agora := novoServicoHistorico(prov)
	ctx := context.Background()
	if _, err := s.GetHistory(ctx, domain.MoedaUSD, 0); err != nil {
		t.Fatal(err)
	}

	prov.err = errors.New("429")
	*agora = agoraBTC.Add(2 * time.Hour)
	h, err := s.GetHistory(ctx, domain.MoedaUSD, 0)
	if err != nil {
		t.Fatalf("erro: %v", err)
	}
	if !h.Stale || !h.UpdatedAt.Equal(agoraBTC) || len(h.Points) == 0 {
		t.Errorf("histórico = stale %v, updated %v, %d pontos", h.Stale, h.UpdatedAt, len(h.Points))
	}
}

func TestHistoricoFalhaSemDadosIndisponivel(t *testing.T) {
	s, _, _ := novoServicoHistorico(&fakeHistProvider{err: errors.New("timeout")})
	if _, err := s.GetHistory(context.Background(), domain.MoedaUSD, 0); !errors.Is(err, domain.ErrIndisponivel) {
		t.Fatalf("erro = %v, queria ErrIndisponivel", err)
	}

	// Diária ok, mas a de hora em hora vazia.
	s, _, _ = novoServicoHistorico(&fakeHistProvider{diarios: pontosDiarios()})
	_, err := s.GetHistory(context.Background(), domain.MoedaUSD, 0)
	if !errors.Is(err, domain.ErrIndisponivel) {
		t.Fatalf("erro = %v, queria ErrIndisponivel", err)
	}

	s, _, _ = novoServicoHistorico(&fakeHistProvider{})
	_, err = s.GetHistory(context.Background(), domain.MoedaUSD, 0)
	if !errors.Is(err, domain.ErrIndisponivel) {
		t.Fatalf("lista vazia: erro = %v, queria ErrIndisponivel", err)
	}
}

func TestHistoricoEntradaInvalida(t *testing.T) {
	prov := novoFakeHistProvider()
	s, _, _ := novoServicoHistorico(prov)
	for _, c := range []struct {
		moeda string
		dias  int
	}{{"eur", 0}, {"", 0}, {"USD", 0}, {"usd", -1}, {"usd", 361}} {
		_, err := s.GetHistory(context.Background(), c.moeda, c.dias)
		if !errors.Is(err, domain.ErrEntradaInvalida) {
			t.Errorf("%q/%d: erro = %v, queria ErrEntradaInvalida", c.moeda, c.dias, err)
		}
	}
	if prov.chamadas.Load() != 0 {
		t.Errorf("entrada inválida chamou a CoinGecko %d vezes", prov.chamadas.Load())
	}
}

func TestHistoricoSincronizacoesSimultaneasSaoUnidas(t *testing.T) {
	prov := novoFakeHistProvider()
	prov.liberar = make(chan struct{})
	s, _, _ := novoServicoHistorico(prov)

	var wg sync.WaitGroup
	erros := make(chan error, 5)
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.GetHistory(context.Background(), domain.MoedaUSD, 0)
			erros <- err
		}()
	}
	// Espera todas entrarem no singleflight antes de liberar a resposta.
	for prov.chamadas.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	time.Sleep(20 * time.Millisecond)
	close(prov.liberar)
	wg.Wait()
	close(erros)

	for err := range erros {
		if err != nil {
			t.Fatalf("erro: %v", err)
		}
	}
	if prov.chamadas.Load() != 2 { // uma sincronização: diária + de hora em hora
		t.Errorf("chamadas = %d, queria 2", prov.chamadas.Load())
	}
}

func TestHistoricoAte360DiasUmPontoPorDia(t *testing.T) {
	prov := novoFakeHistProvider()
	s, repo, _ := novoServicoHistorico(prov)
	ctx := context.Background()

	h, err := s.GetHistory(ctx, domain.MoedaBRL, 360)
	if err != nil {
		t.Fatalf("erro: %v", err)
	}
	if h.Days != 360 || h.Interval != domain.IntervaloDia {
		t.Errorf("days = %d, interval = %q", h.Days, h.Interval)
	}

	// Banco: diária só antes do primeiro ponto de hora em hora, sem repetir instante.
	salvos := repo.pontos[domain.MoedaBRL]
	inicioHora := pontosHoraEmHora()[0].At
	for i, p := range salvos {
		if i > 0 && !p.At.After(salvos[i-1].At) {
			t.Fatalf("salvos fora de ordem em %d", i)
		}
		if p.At.Before(inicioHora) && p.Price != "50000" || !p.At.Before(inicioHora) && p.Price != "67000" {
			t.Fatalf("ponto %v com preço %s da janela errada", p.At, p.Price)
		}
	}

	// Resposta: no máximo um ponto por dia, mais o último (preço de agora).
	for i := 1; i < len(h.Points)-1; i++ {
		if mesmoDiaUTC(h.Points[i-1].At, h.Points[i].At) {
			t.Fatalf("dois pontos no dia %v", h.Points[i].At)
		}
	}
	if n := len(h.Points); n < 360 || n > 362 || !h.Points[n-1].At.Equal(agoraBTC) {
		t.Errorf("pontos = %d, último = %v", n, h.Points[n-1].At)
	}

	// 90 dias ainda sai de hora em hora.
	h, err = s.GetHistory(ctx, domain.MoedaBRL, 90)
	if err != nil {
		t.Fatal(err)
	}
	if h.Interval != domain.IntervaloHora || len(h.Points) != 90*24+1 {
		t.Errorf("90 dias: interval = %q, pontos = %d", h.Interval, len(h.Points))
	}
}
