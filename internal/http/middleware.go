package httpapi

import (
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

// Middleware em Go é só uma função que recebe um http.Handler e devolve outro
// que o "embrulha" — mesma ideia dos interceptors do Dio.

func requestLogger(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			log.InfoContext(r.Context(), "request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.Status(),
				"bytes", ww.BytesWritten(),
				"duracao_ms", time.Since(start).Milliseconds(),
				"ip", r.RemoteAddr,
				"request_id", middleware.GetReqID(r.Context()),
			)
		})
	}
}

// recoverer captura panics (o equivalente a uma exceção não tratada) para que
// um bug num handler não derrube o processo inteiro.
func recoverer(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					if rec == http.ErrAbortHandler {
						panic(rec)
					}
					log.ErrorContext(r.Context(), "panic", "erro", rec, "stack", string(debug.Stack()))
					writeError(w, http.StatusInternalServerError, "erro_interno", "erro interno")
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
