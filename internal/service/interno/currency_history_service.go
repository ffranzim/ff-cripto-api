package interno

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/franzim/ff-cripto-api/internal/domain"
)

const (
	// DiasHistoricoMoedas: cotações buscadas na AwesomeAPI (json/daily/PAR/30) e
	// máximo aceito em ?days. São dias com cotação (úteis), ~6 semanas corridas.
	DiasHistoricoMoedas = 30

	// TTLHistoricoMoedas: os pontos são diários; só o mais recente (agora) muda.
	TTLHistoricoMoedas = time.Hour
)

// CurrencyHistoryProvider é satisfeito pelo *awesomeapi.AwesomeAPIMoedas.
type CurrencyHistoryProvider interface {
	FetchDaily(ctx context.Context, par string, dias int) ([]domain.CurrencyQuote, error)
}

// CurrencyHistoryService serve a cotação diária dos pares de "pares", com cache
// em memória por par (sempre a janela de DiasHistoricoMoedas, recortada por ?days).
// Se a busca falhar, devolve os últimos pontos bons com Stale = true.
type CurrencyHistoryService struct {
	provider CurrencyHistoryProvider
	pares    []string
	source   string
	ttl      time.Duration
	timeout  time.Duration
	agora    func() time.Time

	mu    sync.Mutex
	cache map[string]domain.CurrencyHistory // chave = par

	grupo singleflight.Group
}

// NewCurrencyHistoryService: ttl <= 0 usa TTLHistoricoMoedas; timeout <= 0 usa 5s.
func NewCurrencyHistoryService(provider CurrencyHistoryProvider, pares []string, source string, ttl, timeout time.Duration) *CurrencyHistoryService {
	if ttl <= 0 {
		ttl = TTLHistoricoMoedas
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &CurrencyHistoryService{
		provider: provider, pares: pares, source: source, ttl: ttl, timeout: timeout,
		agora: time.Now, cache: make(map[string]domain.CurrencyHistory, len(pares)),
	}
}

// GetHistory: par fora da lista ou dias fora de 1..30 (0 = 30) -> domain.ErrEntradaInvalida;
// falha sem pontos anteriores -> domain.ErrIndisponivel.
func (s *CurrencyHistoryService) GetHistory(ctx context.Context, par string, dias int) (domain.CurrencyHistory, error) {
	if !slices.Contains(s.pares, par) {
		return domain.CurrencyHistory{}, fmt.Errorf("%w: pair deve ser um de %v", domain.ErrEntradaInvalida, s.pares)
	}
	if dias == 0 {
		dias = DiasHistoricoMoedas
	}
	if dias < 1 || dias > DiasHistoricoMoedas {
		return domain.CurrencyHistory{}, fmt.Errorf("%w: days deve ficar entre 1 e %d", domain.ErrEntradaInvalida, DiasHistoricoMoedas)
	}

	s.mu.Lock()
	h, temCache := s.cache[par]
	s.mu.Unlock()

	if !temCache || s.agora().Sub(h.UpdatedAt) >= s.ttl {
		novo, err := s.buscar(ctx, par)
		switch {
		case err == nil:
			h = novo
		case ctx.Err() != nil:
			return domain.CurrencyHistory{}, ctx.Err()
		case !temCache:
			return domain.CurrencyHistory{}, err
		default:
			h.Stale = true
		}
	}

	// Como na AwesomeAPI, "dias" = últimas N cotações (dias úteis, não corridos).
	// O cache guarda sempre as 30; h é uma cópia, então recortar não o altera.
	if len(h.Points) > dias {
		h.Points = h.Points[len(h.Points)-dias:]
	}
	h.Days = dias
	return h, nil
}

func (s *CurrencyHistoryService) buscar(ctx context.Context, par string) (domain.CurrencyHistory, error) {
	ch := s.grupo.DoChan(par, func() (any, error) {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.timeout)
		defer cancel()

		pontos, err := s.provider.FetchDaily(ctx, par, DiasHistoricoMoedas)
		if err != nil {
			return nil, fmt.Errorf("%w: histórico de %s (%s): %w", domain.ErrIndisponivel, par, s.source, err)
		}
		h := domain.CurrencyHistory{Pair: par, Points: pontos, Source: s.source, UpdatedAt: s.agora().UTC()}
		s.mu.Lock()
		s.cache[par] = h
		s.mu.Unlock()
		return h, nil
	})

	select {
	case r := <-ch:
		if r.Err != nil {
			return domain.CurrencyHistory{}, r.Err
		}
		return r.Val.(domain.CurrencyHistory), nil
	case <-ctx.Done():
		return domain.CurrencyHistory{}, ctx.Err()
	}
}
