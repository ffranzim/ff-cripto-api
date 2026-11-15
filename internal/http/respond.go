package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"

	"github.com/franzim/ff-cripto-api/internal/domain"
)

type erroBody struct {
	Erro erroDetalhe `json:"erro"`
}

type erroDetalhe struct {
	Codigo   string `json:"codigo"`
	Mensagem string `json:"mensagem"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v) // cabeçalho já foi enviado; não há o que fazer com o erro
}

func writeError(w http.ResponseWriter, status int, codigo, msg string) {
	writeJSON(w, status, erroBody{Erro: erroDetalhe{Codigo: codigo, Mensagem: msg}})
}

// handleError traduz erros de domínio em status HTTP. Erros desconhecidos viram 500
// e são logados — a mensagem interna (SQL etc.) nunca vai para o cliente.
func (h *handlers) handleError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, domain.ErrEntradaInvalida):
		writeError(w, http.StatusBadRequest, "entrada_invalida", err.Error())
	case errors.Is(err, domain.ErrNaoEncontrado):
		writeError(w, http.StatusNotFound, "nao_encontrado", err.Error())
	case errors.Is(err, domain.ErrNaoAutorizado):
		writeError(w, http.StatusUnauthorized, "nao_autorizado", err.Error())
	case errors.Is(err, domain.ErrConflito):
		writeError(w, http.StatusConflict, "conflito", err.Error())
	case errors.Is(err, domain.ErrIndisponivel):
		// Loga os motivos (ex.: falha de cada provedor), mas não os expõe ao cliente.
		h.log.WarnContext(r.Context(), "dependência indisponível",
			"err", err, "path", r.URL.Path, "request_id", middleware.GetReqID(r.Context()))
		writeError(w, http.StatusServiceUnavailable, "indisponivel", "serviço temporariamente indisponível")
	default:
		h.log.ErrorContext(r.Context(), "erro interno",
			"err", err, "path", r.URL.Path, "request_id", middleware.GetReqID(r.Context()))
		writeError(w, http.StatusInternalServerError, "erro_interno", "erro interno")
	}
}
