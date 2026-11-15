package interno

import (
	"context"
	"fmt"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/franzim/ff-cripto-api/internal/domain"
)

// BitcoinProviderQuoteService consulta UM provedor específico, sem fallback e sem
// gravar no banco (o histórico continua vindo só do BitcoinQuoteService).
//
// Como as rotas são públicas, cada provedor tem cache em memória pelo TTL e
// buscas simultâneas ao mesmo provedor são unidas (singleflight): assim a rota
// não vira um jeito de gastar a cota das APIs externas.
type BitcoinProviderQuoteService struct {
	providers map[string]BitcoinQuoteProvider // chave = domain.Fonte*
	ttl       time.Duration
	timeout   time.Duration // por busca; igual ao do fallback
	agora     func() time.Time

	mu    sync.Mutex
	cache map[string]domain.BitcoinQuote

	grupo singleflight.Group
}

// NewBitcoinProviderQuoteService: ttl <= 0 usa TTLPadraoCotacaoBitcoin;
// timeout <= 0 usa 5s.
func NewBitcoinProviderQuoteService(providers map[string]BitcoinQuoteProvider, ttl, timeout time.Duration) *BitcoinProviderQuoteService {
	if ttl <= 0 {
		ttl = TTLPadraoCotacaoBitcoin
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &BitcoinProviderQuoteService{
		providers: providers,
		ttl:       ttl,
		timeout:   timeout,
		agora:     time.Now,
		cache:     make(map[string]domain.BitcoinQuote, len(providers)),
	}
}

// GetQuoteFrom devolve a cotação do provedor "fonte".
// Fonte desconhecida -> domain.ErrNaoEncontrado; falha do provedor -> domain.ErrIndisponivel.
func (s *BitcoinProviderQuoteService) GetQuoteFrom(ctx context.Context, fonte string) (domain.BitcoinQuote, error) {
	p, ok := s.providers[fonte]
	if !ok {
		return domain.BitcoinQuote{}, fmt.Errorf("%w: provedor de cotação %q", domain.ErrNaoEncontrado, fonte)
	}

	if q, ok := s.emCache(fonte); ok {
		return q, nil
	}

	// Desacoplada do ctx de quem chegou primeiro (mesmo motivo do BitcoinQuoteService).
	ch := s.grupo.DoChan(fonte, func() (any, error) {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.timeout)
		defer cancel()

		q, err := p.Fetch(ctx)
		if err != nil {
			return nil, fmt.Errorf("%w: %s: %w", domain.ErrIndisponivel, fonte, err)
		}
		s.mu.Lock()
		s.cache[fonte] = q
		s.mu.Unlock()
		return q, nil
	})

	select {
	case r := <-ch:
		if r.Err != nil {
			return domain.BitcoinQuote{}, r.Err
		}
		return r.Val.(domain.BitcoinQuote), nil
	case <-ctx.Done():
		return domain.BitcoinQuote{}, ctx.Err()
	}
}

func (s *BitcoinProviderQuoteService) emCache(fonte string) (domain.BitcoinQuote, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	q, ok := s.cache[fonte]
	if !ok || s.agora().Sub(q.FetchedAt) >= s.ttl {
		return domain.BitcoinQuote{}, false
	}
	return q, true
}
