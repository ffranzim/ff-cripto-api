package interno

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/franzim/ff-cripto-api/internal/domain"
)

const (
	// DiasHistoricoBitcoin: máximo aceito em ?days e janela diária buscada na
	// CoinGecko a cada sincronização. Acima de 90 dias ela só devolve um ponto
	// por dia (00:00 UTC); a API demo/pública vai até 365 dias.
	DiasHistoricoBitcoin = 360

	// DiasHistoricoBitcoinPorHora: janela buscada com um ponto por hora (a
	// CoinGecko dá hora em hora de 2 a 90 dias). Pedidos de até esse número de
	// dias saem de hora em hora; acima, um ponto por dia.
	DiasHistoricoBitcoinPorHora = 90

	// DiasHistoricoBitcoinPadrao: ?days ausente.
	DiasHistoricoBitcoinPadrao = 30

	// TTLHistoricoBitcoin: os pontos são de hora em hora, então sincronizar mais
	// que isso só gasta a cota da CoinGecko (4 chamadas por hora: diária e de
	// hora em hora, para cada moeda).
	TTLHistoricoBitcoin = time.Hour

	timeoutSincronizacaoHistorico = 30 * time.Second
)

// BitcoinHistoryProvider é satisfeito pelo *coingecko.CoinGeckoHistorico.
type BitcoinHistoryProvider interface {
	FetchHistory(ctx context.Context, moeda string, dias int) ([]domain.BitcoinHistoryPoint, error)
}

// BitcoinHistoryRepository é satisfeito pelo *repository.BitcoinHistoryRepository.
type BitcoinHistoryRepository interface {
	Replace(ctx context.Context, moeda string, pontos []domain.BitcoinHistoryPoint) error
	List(ctx context.Context, moeda string, desde time.Time) ([]domain.BitcoinHistoryPoint, error)
	LastSync(ctx context.Context, moeda string) (time.Time, bool, error)
}

// BitcoinHistoryService serve o histórico do banco e o sincroniza com a
// CoinGecko quando a última sincronização daquela moeda passou do TTL.
type BitcoinHistoryService struct {
	provider BitcoinHistoryProvider
	repo     BitcoinHistoryRepository
	ttl      time.Duration
	agora    func() time.Time

	// Uma sincronização por moeda de cada vez; as requests simultâneas esperam por ela.
	grupo singleflight.Group
}

// NewBitcoinHistoryService: ttl <= 0 usa TTLHistoricoBitcoin.
func NewBitcoinHistoryService(provider BitcoinHistoryProvider, repo BitcoinHistoryRepository, ttl time.Duration) *BitcoinHistoryService {
	if ttl <= 0 {
		ttl = TTLHistoricoBitcoin
	}
	return &BitcoinHistoryService{provider: provider, repo: repo, ttl: ttl, agora: time.Now}
}

// GetHistory devolve os pontos dos últimos "dias" (0 = DiasHistoricoBitcoinPadrao)
// em "moeda" (usd | brl): de hora em hora até DiasHistoricoBitcoinPorHora dias,
// um por dia acima disso. Se a sincronização falhar, devolve o que estiver salvo
// com Stale = true; sem nada salvo, devolve domain.ErrIndisponivel.
func (s *BitcoinHistoryService) GetHistory(ctx context.Context, moeda string, dias int) (domain.BitcoinHistory, error) {
	if moeda != domain.MoedaUSD && moeda != domain.MoedaBRL {
		return domain.BitcoinHistory{}, fmt.Errorf("%w: currency deve ser usd ou brl", domain.ErrEntradaInvalida)
	}
	if dias == 0 {
		dias = DiasHistoricoBitcoinPadrao
	}
	if dias < 1 || dias > DiasHistoricoBitcoin {
		return domain.BitcoinHistory{}, fmt.Errorf("%w: days deve ficar entre 1 e %d", domain.ErrEntradaInvalida, DiasHistoricoBitcoin)
	}

	sincronizado, ok, err := s.repo.LastSync(ctx, moeda)
	if err != nil {
		return domain.BitcoinHistory{}, err
	}

	stale := false
	if !ok || s.agora().Sub(sincronizado) >= s.ttl {
		quando, errSync := s.sincronizar(ctx, moeda)
		switch {
		case errSync == nil:
			sincronizado = quando
		case ctx.Err() != nil:
			return domain.BitcoinHistory{}, ctx.Err()
		case !ok:
			return domain.BitcoinHistory{}, errSync
		default:
			stale = true // serve o que está salvo
		}
	}

	pontos, err := s.repo.List(ctx, moeda, s.agora().Add(-time.Duration(dias)*24*time.Hour))
	if err != nil {
		return domain.BitcoinHistory{}, err
	}
	intervalo := domain.IntervaloHora
	if dias > DiasHistoricoBitcoinPorHora {
		intervalo = domain.IntervaloDia
		pontos = umPorDia(pontos)
	}
	return domain.BitcoinHistory{Currency: moeda, Days: dias, Interval: intervalo, Points: pontos, UpdatedAt: sincronizado, Stale: stale}, nil
}

// umPorDia fica com o primeiro ponto de cada dia (UTC) e com o último ponto,
// para o gráfico terminar no preço mais recente. Os pontos vêm em ordem.
func umPorDia(pontos []domain.BitcoinHistoryPoint) []domain.BitcoinHistoryPoint {
	out := make([]domain.BitcoinHistoryPoint, 0, len(pontos)/24+2)
	for i, p := range pontos {
		if len(out) == 0 || !mesmoDiaUTC(out[len(out)-1].At, p.At) || i == len(pontos)-1 {
			out = append(out, p)
		}
	}
	return out
}

func mesmoDiaUTC(a, b time.Time) bool {
	ay, am, ad := a.UTC().Date()
	by, bm, bd := b.UTC().Date()
	return ay == by && am == bm && ad == bd
}

// sincronizar busca na CoinGecko a janela diária (DiasHistoricoBitcoin) e a de
// hora em hora (DiasHistoricoBitcoinPorHora), junta as duas (diária só antes do
// primeiro ponto de hora em hora) e substitui a janela do banco.
// Desacoplada do ctx de quem chegou primeiro (mesmo motivo do BitcoinQuoteService).
func (s *BitcoinHistoryService) sincronizar(ctx context.Context, moeda string) (time.Time, error) {
	ch := s.grupo.DoChan(moeda, func() (any, error) {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeoutSincronizacaoHistorico)
		defer cancel()

		diarios, err := s.provider.FetchHistory(ctx, moeda, DiasHistoricoBitcoin)
		if err != nil {
			return nil, fmt.Errorf("%w: histórico diário do bitcoin (%s): %w", domain.ErrIndisponivel, moeda, err)
		}
		porHora, err := s.provider.FetchHistory(ctx, moeda, DiasHistoricoBitcoinPorHora)
		if err != nil {
			return nil, fmt.Errorf("%w: histórico por hora do bitcoin (%s): %w", domain.ErrIndisponivel, moeda, err)
		}
		if len(porHora) == 0 {
			return nil, fmt.Errorf("%w: histórico do bitcoin (%s) vazio", domain.ErrIndisponivel, moeda)
		}

		pontos := make([]domain.BitcoinHistoryPoint, 0, len(diarios)+len(porHora))
		for _, p := range diarios {
			if p.At.Before(porHora[0].At) {
				pontos = append(pontos, p)
			}
		}
		pontos = append(pontos, porHora...)
		if err := s.repo.Replace(ctx, moeda, pontos); err != nil {
			return nil, err
		}
		return s.agora().UTC(), nil
	})

	select {
	case r := <-ch:
		if r.Err != nil {
			return time.Time{}, r.Err
		}
		return r.Val.(time.Time), nil
	case <-ctx.Done():
		return time.Time{}, ctx.Err()
	}
}
