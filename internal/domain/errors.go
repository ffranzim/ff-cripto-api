package domain

import "errors"

// Erros "sentinela". Em Go erros são valores (não exceções): as camadas de baixo
// os embrulham com fmt.Errorf("...: %w", err) e a camada HTTP pergunta
// errors.Is(err, domain.ErrNaoEncontrado) — parecido com um `on NotFoundException` no Dart.
var (
	ErrNaoEncontrado   = errors.New("recurso não encontrado")
	ErrEntradaInvalida = errors.New("entrada inválida")
	ErrConflito        = errors.New("conflito")
	ErrNaoAutorizado   = errors.New("não autorizado")
	// ErrIndisponivel: dependência externa fora do ar (ex.: todos os provedores
	// de cotação falharam); a camada HTTP responde 503.
	ErrIndisponivel = errors.New("serviço indisponível")
)
