package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"financial-ledger-engine/internal/domain"
	transportHttp "financial-ledger-engine/internal/transport/http"
)

// mockLedgerService implementa a interface transportHttp.LedgerService para testes unitários isolados
type mockLedgerService struct {
	createAccountFunc      func(ctx context.Context, acc *domain.Account) (*domain.Account, error)
	getAccountByIDFunc     func(ctx context.Context, id string) (*domain.Account, error)
	getAccountBalanceFunc  func(ctx context.Context, accountID string) (*domain.AccountBalance, error)
	executeTransactionFunc func(ctx context.Context, tx *domain.Transaction) (*domain.Transaction, bool, error)
}

func (m *mockLedgerService) CreateAccount(ctx context.Context, acc *domain.Account) (*domain.Account, error) {
	if m.createAccountFunc != nil {
		return m.createAccountFunc(ctx, acc)
	}
	return acc, nil
}

func (m *mockLedgerService) GetAccountByID(ctx context.Context, id string) (*domain.Account, error) {
	if m.getAccountByIDFunc != nil {
		return m.getAccountByIDFunc(ctx, id)
	}
	return &domain.Account{ID: id, Name: "Conta Teste", Type: domain.AccountTypeAsset, Currency: "BRL"}, nil
}

func (m *mockLedgerService) GetAccountBalance(ctx context.Context, accountID string) (*domain.AccountBalance, error) {
	if m.getAccountBalanceFunc != nil {
		return m.getAccountBalanceFunc(ctx, accountID)
	}
	return &domain.AccountBalance{
		AccountID:        accountID,
		AccountName:      "Conta Caixa",
		AccountType:      domain.AccountTypeAsset,
		Currency:         "BRL",
		Balance:          10000,
		BalanceFormatted: "BRL 100.00",
		TotalDebit:       15000,
		TotalCredit:      5000,
		UpdatedAt:        time.Now(),
	}, nil
}

func (m *mockLedgerService) ExecuteTransaction(ctx context.Context, tx *domain.Transaction) (*domain.Transaction, bool, error) {
	if m.executeTransactionFunc != nil {
		return m.executeTransactionFunc(ctx, tx)
	}
	tx.ID = "tx-123"
	tx.PostedAt = time.Now()
	tx.CreatedAt = time.Now()
	return tx, false, nil
}

func TestHealthCheck(t *testing.T) {
	handler := transportHttp.NewHandler(&mockLedgerService{})
	router := transportHttp.NewRouter(handler)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("esperado status 200, recebido %d", rr.Code)
	}
}

func TestCreateAccount_Success(t *testing.T) {
	mockSvc := &mockLedgerService{
		createAccountFunc: func(ctx context.Context, acc *domain.Account) (*domain.Account, error) {
			acc.ID = "acc-uuid-1"
			acc.CreatedAt = time.Now()
			return acc, nil
		},
	}

	handler := transportHttp.NewHandler(mockSvc)
	router := transportHttp.NewRouter(handler)

	payload := transportHttp.CreateAccountRequest{
		Name:     "Caixa Matriz",
		Type:     domain.AccountTypeAsset,
		Currency: "BRL",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("esperado status 201, recebido %d", rr.Code)
	}

	var resp domain.Account
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("falha ao decodificar resposta: %v", err)
	}
	if resp.ID != "acc-uuid-1" || resp.Name != "Caixa Matriz" {
		t.Errorf("dados inesperados da conta: %+v", resp)
	}
}

func TestCreateAccount_InvalidType(t *testing.T) {
	mockSvc := &mockLedgerService{
		createAccountFunc: func(ctx context.Context, acc *domain.Account) (*domain.Account, error) {
			return nil, domain.ErrInvalidAccountType
		},
	}

	handler := transportHttp.NewHandler(mockSvc)
	router := transportHttp.NewRouter(handler)

	payload := map[string]string{
		"name":     "Conta Invalida",
		"type":     "TIPO_INEXISTENTE",
		"currency": "BRL",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("esperado status 400, recebido %d", rr.Code)
	}
}

func TestExecuteTransaction_MissingIdempotencyKey(t *testing.T) {
	handler := transportHttp.NewHandler(&mockLedgerService{})
	router := transportHttp.NewRouter(handler)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/transactions", bytes.NewReader([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")
	// Sem header Idempotency-Key
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("esperado status 400 para falta de Idempotency-Key, recebido %d", rr.Code)
	}
}

func TestExecuteTransaction_Success(t *testing.T) {
	mockSvc := &mockLedgerService{
		executeTransactionFunc: func(ctx context.Context, tx *domain.Transaction) (*domain.Transaction, bool, error) {
			tx.ID = "tx-12345"
			tx.PostedAt = time.Now()
			tx.CreatedAt = time.Now()
			return tx, false, nil
		},
	}

	handler := transportHttp.NewHandler(mockSvc)
	router := transportHttp.NewRouter(handler)

	payload := transportHttp.CreateTransactionRequest{
		Description: "Aporte inicial",
		Entries: []domain.Entry{
			{AccountID: "acc-1", Amount: 5000, Type: domain.EntryTypeDebit},
			{AccountID: "acc-2", Amount: 5000, Type: domain.EntryTypeCredit},
		},
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/transactions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "idem-uuid-1")
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("esperado status 201, recebido %d", rr.Code)
	}

	var resp domain.Transaction
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("falha ao decodificar resposta: %v", err)
	}
	if resp.ID != "tx-12345" || resp.Reference != "idem-uuid-1" {
		t.Errorf("transação inesperada: %+v", resp)
	}
}

func TestExecuteTransaction_IdempotentReplay(t *testing.T) {
	mockSvc := &mockLedgerService{
		executeTransactionFunc: func(ctx context.Context, tx *domain.Transaction) (*domain.Transaction, bool, error) {
			tx.ID = "tx-previously-created"
			return tx, true, nil // isReplay = true
		},
	}

	handler := transportHttp.NewHandler(mockSvc)
	router := transportHttp.NewRouter(handler)

	payload := transportHttp.CreateTransactionRequest{
		Description: "Aporte inicial",
		Entries: []domain.Entry{
			{AccountID: "acc-1", Amount: 5000, Type: domain.EntryTypeDebit},
			{AccountID: "acc-2", Amount: 5000, Type: domain.EntryTypeCredit},
		},
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/transactions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "idem-uuid-1")
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("esperado status 200 para replay idempotente, recebido %d", rr.Code)
	}
	if rr.Header().Get("Idempotent-Replay") != "true" {
		t.Errorf("esperado cabeçalho Idempotent-Replay = true")
	}
}

func TestExecuteTransaction_Unbalanced(t *testing.T) {
	mockSvc := &mockLedgerService{
		executeTransactionFunc: func(ctx context.Context, tx *domain.Transaction) (*domain.Transaction, bool, error) {
			return nil, false, domain.ErrUnbalancedTransaction
		},
	}

	handler := transportHttp.NewHandler(mockSvc)
	router := transportHttp.NewRouter(handler)

	payload := transportHttp.CreateTransactionRequest{
		Description: "Transação desbalanceada",
		Entries: []domain.Entry{
			{AccountID: "acc-1", Amount: 5000, Type: domain.EntryTypeDebit},
			{AccountID: "acc-2", Amount: 4000, Type: domain.EntryTypeCredit},
		},
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/transactions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "idem-fail-unbalanced")
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("esperado status 422 para desbalanceada, recebido %d", rr.Code)
	}
}

func TestExecuteTransaction_InsufficientBalance(t *testing.T) {
	mockSvc := &mockLedgerService{
		executeTransactionFunc: func(ctx context.Context, tx *domain.Transaction) (*domain.Transaction, bool, error) {
			return nil, false, domain.ErrInsufficientBalance
		},
	}

	handler := transportHttp.NewHandler(mockSvc)
	router := transportHttp.NewRouter(handler)

	payload := transportHttp.CreateTransactionRequest{
		Description: "Saque sem saldo",
		Entries: []domain.Entry{
			{AccountID: "acc-1", Amount: 999999, Type: domain.EntryTypeCredit},
			{AccountID: "acc-2", Amount: 999999, Type: domain.EntryTypeDebit},
		},
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/transactions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "idem-overdraft")
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("esperado status 422 para saldo insuficiente, recebido %d", rr.Code)
	}
}

func TestGetAccountBalance_Success(t *testing.T) {
	mockSvc := &mockLedgerService{
		getAccountBalanceFunc: func(ctx context.Context, accountID string) (*domain.AccountBalance, error) {
			return &domain.AccountBalance{
				AccountID:        accountID,
				AccountName:      "Banco Conta Corrente",
				AccountType:      domain.AccountTypeAsset,
				Currency:         "BRL",
				Balance:          250000,
				BalanceFormatted: "BRL 2500.00",
				TotalDebit:       300000,
				TotalCredit:      50000,
				UpdatedAt:        time.Now(),
			}, nil
		},
	}

	handler := transportHttp.NewHandler(mockSvc)
	router := transportHttp.NewRouter(handler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/acc-123/balance", nil)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("esperado status 200, recebido %d", rr.Code)
	}

	var bal domain.AccountBalance
	if err := json.NewDecoder(rr.Body).Decode(&bal); err != nil {
		t.Fatalf("falha ao decodificar saldo: %v", err)
	}
	if bal.Balance != 250000 || bal.AccountName != "Banco Conta Corrente" {
		t.Errorf("dados de saldo inesperados: %+v", bal)
	}
}

func TestGetAccountBalance_NotFound(t *testing.T) {
	mockSvc := &mockLedgerService{
		getAccountBalanceFunc: func(ctx context.Context, accountID string) (*domain.AccountBalance, error) {
			return nil, domain.ErrAccountNotFound
		},
	}

	handler := transportHttp.NewHandler(mockSvc)
	router := transportHttp.NewRouter(handler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/acc-unknown/balance", nil)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("esperado status 404 para conta não encontrada, recebido %d", rr.Code)
	}
}

