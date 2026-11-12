package http

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// NewRouter configura o roteador Chi com middlewares de observabilidade e rotas da API.
func NewRouter(handler *Handler) http.Handler {
	r := chi.NewRouter()

	// Middlewares essenciais de resiliência e observabilidade
	r.Use(middleware.RequestID)                 // Identificador único de correlação para tracing
	r.Use(middleware.RealIP)                    // Captura o IP real do cliente
	r.Use(middleware.Logger)                    // Logs estruturados para cada requisição
	r.Use(middleware.Recoverer)                 // Tratamento global de panics prevenindo crash da API
	r.Use(middleware.Timeout(30 * time.Second)) // Timeout global de 30s por requisição

	// Healthcheck para orquestradores (Kubernetes liveness/readiness, Docker healthcheck)
	r.Get("/healthz", handler.HealthCheck)

	// Rotas da API V1
	r.Route("/api/v1", func(r chi.Router) {
		// Gerenciamento de Contas Contábeis
		r.Route("/accounts", func(r chi.Router) {
			r.Post("/", handler.CreateAccount)
			r.Get("/{id}", handler.GetAccountByID)
			r.Get("/{id}/balance", handler.GetAccountBalance)
		})

		// Execução de Transações de Partidas Dobradas com Idempotência
		r.Route("/transactions", func(r chi.Router) {
			r.Post("/", handler.ExecuteTransaction)
		})
	})

	return r
}
