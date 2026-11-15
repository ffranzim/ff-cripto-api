package interno

import (
	"context"
	"fmt"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/franzim/ff-cripto-api/internal/domain"
)

// CurrencyQuoteProvider é satisfeito pelo *awesomeapi.AwesomeAPIMoedas.
type CurrencyQuoteProvider interface {
	FetchCurrencies(ctx context.Context) ([]domain.CurrencyQuote, error)
}

// CurrencyQuoteService serve as cotações de moedas com cache em memória pelo TTL.
// A rota é pública: o cache e o singleflight evitam gastar a cota da AwesomeAPI.
// Se a busca falhar, devolve as últimas cotações boas com Stale = true.
type CurrencyQuoteService struct {
	provider CurrencyQuoteProvider
	source   string
	ttl      time.Duration
	timeout  time.Duration
	agora    func() time.Time

	mu     sync.Mutex
	ultima *domain.CurrencyQuotesResult

	grupo singleflight.Group
}

// NewCurrencyQuoteService: ttl <= 0 usa TTLPadraoCotacaoBitcoin; timeout <= 0 usa 5s.
func NewCurrencyQuoteService(provider CurrencyQuoteProvider, source string, ttl, timeout time.Duration) *CurrencyQuoteService {
	if ttl <= 0 {
		ttl = TTLPadraoCotacaoBitcoin
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &CurrencyQuoteService{provider: provider, source: source, ttl: ttl, timeout: timeout, agora: time.Now}
}

// GetQuotes: falha sem nenhuma cotação boa anterior -> domain.ErrIndisponivel.
func (s *CurrencyQuoteService) GetQuotes(ctx context.Context) (domain.CurrencyQuotesResult, error) {
	s.mu.Lock()
	ultima := s.ultima
	s.mu.Unlock()
	if ultima != nil && s.agora().Sub(ultima.FetchedAt) < s.ttl {
		return *ultima, nil
	}

	// Desacoplada do ctx de quem chegou primeiro (mesmo motivo do BitcoinQuoteService).
	ch := s.grupo.DoChan("moedas", func() (any, error) {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.timeout)
		defer cancel()

		quotes, err := s.provider.FetchCurrencies(ctx)
		if err != nil {
			return nil, fmt.Errorf("%w: cotação de moedas (%s): %w", domain.ErrIndisponivel, s.source, err)
		}
		r := domain.CurrencyQuotesResult{Quotes: quotes, Source: s.source, FetchedAt: s.agora().UTC()}
		s.mu.Lock()
		s.ultima = &r
		s.mu.Unlock()
		return r, nil
	})

	select {
	case r := <-ch:
		if r.Err == nil {
			return r.Val.(domain.CurrencyQuotesResult), nil
		}
		if ultima != nil {
			velha := *ultima
			velha.Stale = true
			return velha, nil
		}
		return domain.CurrencyQuotesResult{}, r.Err
	case <-ctx.Done():
		return domain.CurrencyQuotesResult{}, ctx.Err()
	}
}
