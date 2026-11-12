package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"financial-ledger-engine/internal/domain"

	"github.com/go-chi/chi/v5"
)

// LedgerService define a interface de operações do motor contábil para os handlers HTTP.
type LedgerService interface {
	CreateAccount(ctx context.Context, acc *domain.Account) (*domain.Account, error)
	GetAccountByID(ctx context.Context, id string) (*domain.Account, error)
	GetAccountBalance(ctx context.Context, accountID string) (*domain.AccountBalance, error)
	ExecuteTransaction(ctx context.Context, tx *domain.Transaction) (*domain.Transaction, bool, error)
}

// Handler agrupa os controladores HTTP da API do Ledger.
type Handler struct {
	service LedgerService
}

// NewHandler constrói uma nova instância do Handler HTTP.
func NewHandler(service LedgerService) *Handler {
	return &Handler{service: service}
}

// HealthCheck responde com o status de integridade do serviço.
func (h *Handler) HealthCheck(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"status":    "healthy",
		"service":   "financial-ledger-engine",
		"timestamp": time.Now().UTC(),
	})
}

// CreateAccountRequest define o payload para criação de uma nova conta contábil.
type CreateAccountRequest struct {
	Name     string             `json:"name"`
	Type     domain.AccountType `json:"type"`
	Currency string             `json:"currency"`
}

// CreateAccount manipula POST /api/v1/accounts
func (h *Handler) CreateAccount(w http.ResponseWriter, r *http.Request) {
	var req CreateAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{
			"error":  "Payload JSON inválido",
			"detail": err.Error(),
		})
		return
	}

	currency := strings.ToUpper(strings.TrimSpace(req.Currency))
	if currency == "" {
		currency = "BRL"
	}

	acc := &domain.Account{
		Name:     strings.TrimSpace(req.Name),
		Type:     req.Type,
		Currency: currency,
	}

	created, err := h.service.CreateAccount(r.Context(), acc)
	if err != nil {
		if errors.Is(err, domain.ErrInvalidAccountName) ||
			errors.Is(err, domain.ErrInvalidAccountType) ||
			errors.Is(err, domain.ErrInvalidCurrency) {
			respondJSON(w, http.StatusBadRequest, map[string]string{
				"error":  "Erro de validação dos dados da conta",
				"detail": err.Error(),
			})
			return
		}

		respondJSON(w, http.StatusInternalServerError, map[string]string{
			"error":  "Falha interna ao criar conta contábil",
			"detail": err.Error(),
		})
		return
	}

	respondJSON(w, http.StatusCreated, created)
}

// CreateTransactionRequest representa o payload de envio de uma transação contábil.
type CreateTransactionRequest struct {
	Description string         `json:"description"`
	Entries     []domain.Entry `json:"entries"`
}

// ExecuteTransaction manipula POST /api/v1/transactions
// Exige o cabeçalho "Idempotency-Key" para garantir atomicidade e idempotência.
func (h *Handler) ExecuteTransaction(w http.ResponseWriter, r *http.Request) {
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if idempotencyKey == "" {
		// Aceita também X-Idempotency-Key alternativamente
		idempotencyKey = strings.TrimSpace(r.Header.Get("X-Idempotency-Key"))
	}

	if idempotencyKey == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{
			"error":  "Cabeçalho Idempotency-Key é obrigatório",
			"detail": "Todas as transações do Ledger exigem a chave Idempotency-Key no cabeçalho HTTP para garantir que não ocorra processamento duplicado",
		})
		return
	}

	var req CreateTransactionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{
			"error":  "Payload JSON inválido",
			"detail": err.Error(),
		})
		return
	}

	tx := &domain.Transaction{
		Reference:   idempotencyKey,
		Description: req.Description,
		Entries:     req.Entries,
	}

	executedTx, isReplay, err := h.service.ExecuteTransaction(r.Context(), tx)
	if err != nil {
		if errors.Is(err, domain.ErrUnbalancedTransaction) ||
			errors.Is(err, domain.ErrInsufficientEntries) ||
			errors.Is(err, domain.ErrInvalidAmount) ||
			errors.Is(err, domain.ErrInsufficientBalance) {
			respondJSON(w, http.StatusUnprocessableEntity, map[string]string{
				"error":  "Violação de regra contábil",
				"detail": err.Error(),
			})
			return
		}

		if errors.Is(err, domain.ErrAccountNotFound) {
			respondJSON(w, http.StatusNotFound, map[string]string{
				"error":  "Conta não encontrada",
				"detail": err.Error(),
			})
			return
		}

		respondJSON(w, http.StatusInternalServerError, map[string]string{
			"error":  "Falha interna ao processar transação contábil",
			"detail": err.Error(),
		})
		return
	}

	if isReplay {
		w.Header().Set("Idempotent-Replay", "true")
		respondJSON(w, http.StatusOK, map[string]interface{}{
			"message":           "Transação idempotente já processada anteriormente (replay)",
			"idempotent_replay": true,
			"transaction":       executedTx,
		})
		return
	}

	respondJSON(w, http.StatusCreated, executedTx)
}

// GetAccountBalance manipula GET /api/v1/accounts/{id}/balance
func (h *Handler) GetAccountBalance(w http.ResponseWriter, r *http.Request) {
	accountID := strings.TrimSpace(chi.URLParam(r, "id"))
	if accountID == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{
			"error": "ID da conta contábil não informado na URL",
		})
		return
	}

	balance, err := h.service.GetAccountBalance(r.Context(), accountID)
	if err != nil {
		if errors.Is(err, domain.ErrAccountNotFound) {
			respondJSON(w, http.StatusNotFound, map[string]string{
				"error":      "Conta contábil não encontrada",
				"account_id": accountID,
			})
			return
		}

		respondJSON(w, http.StatusInternalServerError, map[string]string{
			"error":  "Falha interna ao consultar saldo da conta",
			"detail": err.Error(),
		})
		return
	}

	respondJSON(w, http.StatusOK, balance)
}

// GetAccountByID manipula GET /api/v1/accounts/{id}
func (h *Handler) GetAccountByID(w http.ResponseWriter, r *http.Request) {
	accountID := strings.TrimSpace(chi.URLParam(r, "id"))
	if accountID == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{
			"error": "ID da conta contábil não informado na URL",
		})
		return
	}

	acc, err := h.service.GetAccountByID(r.Context(), accountID)
	if err != nil {
		if errors.Is(err, domain.ErrAccountNotFound) {
			respondJSON(w, http.StatusNotFound, map[string]string{
				"error":      "Conta contábil não encontrada",
				"account_id": accountID,
			})
			return
		}

		respondJSON(w, http.StatusInternalServerError, map[string]string{
			"error":  "Falha ao consultar detalhes da conta",
			"detail": err.Error(),
		})
		return
	}

	respondJSON(w, http.StatusOK, acc)
}

// respondJSON é o helper idiomático para serialização e envio de cabeçalhos HTTP.
func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
