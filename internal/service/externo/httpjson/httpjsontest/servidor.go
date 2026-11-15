// Package httpjsontest tem o servidor HTTP falso usado nos testes dos clientes externos.
package httpjsontest

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// Servidor responde sempre com o status e o corpo dados e guarda a última request.
func Servidor(t *testing.T, status int, corpo string, ultima **http.Request) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ultima != nil {
			*ultima = r
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(corpo))
	}))
	t.Cleanup(srv.Close)
	return srv
}
