package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"

	"financial-ledger-engine/internal/domain"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// Repository gerencia o acesso aos dados persistidos no PostgreSQL com garantias ACID.
type Repository struct {
	db *sql.DB
}

// NewRepository cria uma nova instância de repositório PostgreSQL com pool de conexões configurado.
func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// OpenDB abre o banco de dados PostgreSQL e configura o pool de conexões para alta performance.
func OpenDB(databaseURL string) (*sql.DB, error) {
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("falha ao abrir conexão com postgres: %w", err)
	}

	// Configuração do pool de conexões (Connection Pooling)
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(15 * time.Minute)
	db.SetConnMaxIdleTime(5 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("falha ao verificar conectividade (ping) com o banco de dados: %w", err)
	}

	return db, nil
}

// InitSchema executa o DDL inicial para garantir que tabelas e extensões existam.
func (r *Repository) InitSchema(ctx context.Context) error {
	schema := `
	CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

	CREATE TABLE IF NOT EXISTS accounts (
		id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
		name VARCHAR(255) NOT NULL,
		type VARCHAR(20) NOT NULL CHECK (type IN ('ASSET', 'LIABILITY', 'EQUITY', 'REVENUE', 'EXPENSE')),
		currency VARCHAR(3) NOT NULL DEFAULT 'BRL',
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);

	CREATE TABLE IF NOT EXISTS transactions (
		id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
		reference VARCHAR(255) UNIQUE,
		description TEXT NOT NULL,
		posted_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);

	CREATE TABLE IF NOT EXISTS entries (
		id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
		transaction_id UUID NOT NULL REFERENCES transactions(id) ON DELETE RESTRICT,
		account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
		amount BIGINT NOT NULL CHECK (amount > 0),
		type VARCHAR(10) NOT NULL CHECK (type IN ('DEBIT', 'CREDIT')),
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);

	CREATE INDEX IF NOT EXISTS idx_entries_transaction_id ON entries(transaction_id);
	CREATE INDEX IF NOT EXISTS idx_entries_account_id ON entries(account_id);
	`
	_, err := r.db.ExecContext(ctx, schema)
	if err != nil {
		return fmt.Errorf("falha ao inicializar schema do banco: %w", err)
	}
	return nil
}

// CreateAccount persiste uma nova conta contábil.
func (r *Repository) CreateAccount(ctx context.Context, acc *domain.Account) (*domain.Account, error) {
	if err := acc.Validate(); err != nil {
		return nil, err
	}

	if acc.ID == "" {
		acc.ID = uuid.New().String()
	}
	if acc.CreatedAt.IsZero() {
		acc.CreatedAt = time.Now().UTC()
	}

	query := `
		INSERT INTO accounts (id, name, type, currency, created_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, name, type, currency, created_at;
	`
	row := r.db.QueryRowContext(ctx, query, acc.ID, acc.Name, acc.Type, acc.Currency, acc.CreatedAt)

	var saved domain.Account
	var accType string
	if err := row.Scan(&saved.ID, &saved.Name, &accType, &saved.Currency, &saved.CreatedAt); err != nil {
		return nil, fmt.Errorf("falha ao persistir conta contábil: %w", err)
	}
	saved.Type = domain.AccountType(accType)
	return &saved, nil
}

// GetAccountByID busca uma conta pelo ID.
func (r *Repository) GetAccountByID(ctx context.Context, id string) (*domain.Account, error) {
	query := `SELECT id, name, type, currency, created_at FROM accounts WHERE id = $1;`
	row := r.db.QueryRowContext(ctx, query, id)

	var acc domain.Account
	var accType string
	if err := row.Scan(&acc.ID, &acc.Name, &accType, &acc.Currency, &acc.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrAccountNotFound
		}
		return nil, fmt.Errorf("falha ao consultar conta contábil: %w", err)
	}
	acc.Type = domain.AccountType(accType)
	return &acc, nil
}

// GetAccountBalance calcula e retorna o saldo consolidado de uma conta contábil.
func (r *Repository) GetAccountBalance(ctx context.Context, accountID string) (*domain.AccountBalance, error) {
	query := `
		SELECT 
			a.id,
			a.name,
			a.type,
			a.currency,
			COALESCE(SUM(CASE WHEN e.type = 'DEBIT' THEN e.amount ELSE 0 END), 0) AS total_debit,
			COALESCE(SUM(CASE WHEN e.type = 'CREDIT' THEN e.amount ELSE 0 END), 0) AS total_credit
		FROM accounts a
		LEFT JOIN entries e ON a.id = e.account_id
		WHERE a.id = $1
		GROUP BY a.id, a.name, a.type, a.currency;
	`
	row := r.db.QueryRowContext(ctx, query, accountID)

	var (
		id          string
		name        string
		accTypeStr  string
		currency    string
		totalDebit  int64
		totalCredit int64
	)

	if err := row.Scan(&id, &name, &accTypeStr, &currency, &totalDebit, &totalCredit); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrAccountNotFound
		}
		return nil, fmt.Errorf("falha ao consultar saldo da conta: %w", err)
	}

	accType := domain.AccountType(accTypeStr)
	balance := domain.CalculateBalance(accType, totalDebit, totalCredit)

	return &domain.AccountBalance{
		AccountID:        id,
		AccountName:      name,
		AccountType:      accType,
		Currency:         currency,
		Balance:          balance,
		BalanceFormatted: domain.FormatMoney(balance, currency),
		TotalDebit:       totalDebit,
		TotalCredit:      totalCredit,
		UpdatedAt:        time.Now().UTC(),
	}, nil
}

// ExecuteTransaction realiza a gravação atômica da transação contábil garantindo:
// 1. Partidas dobradas (débitos == créditos e no mínimo 2 pernas)
// 2. Idempotência estrita via Reference (Idempotency-Key)
// 3. Concorrência segura (bloqueio de linha FOR UPDATE ordenado lexicograficamente para evitar deadlocks)
// 4. Verificação de saldo suficiente (sem saldo negativo em contas)
// 5. Atomicidade completa: BEGIN, COMMIT e ROLLBACK garantido em falhas
func (r *Repository) ExecuteTransaction(ctx context.Context, tx *domain.Transaction) (*domain.Transaction, bool, error) {
	if err := tx.Validate(); err != nil {
		return nil, false, err
	}

	// 1. Inicia transação PostgreSQL explícita (BEGIN)
	dbTx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, false, fmt.Errorf("falha ao iniciar transação no banco de dados: %w", err)
	}
	defer func() {
		_ = dbTx.Rollback() // Seguro: se já efetuado o Commit, é no-op
	}()

	// 2. Garantia de Idempotência: verifica se transação com esta referência já existe
	if tx.Reference != "" {
		existing, err := r.findTransactionByReferenceTx(ctx, dbTx, tx.Reference)
		if err != nil {
			return nil, false, err
		}
		if existing != nil {
			// Idempotência confirmada: retorna a transação gravada anteriormente (replay)
			return existing, true, nil
		}
	}

	// 3. Extrai IDs únicos de contas e ordena para evitar DEADLOCK
	accountIDMap := make(map[string]bool)
	for _, entry := range tx.Entries {
		accountIDMap[entry.AccountID] = true
	}
	accountIDs := make([]string, 0, len(accountIDMap))
	for id := range accountIDMap {
		accountIDs = append(accountIDs, id)
	}
	sort.Strings(accountIDs) // Ordenação lexicográfica garante mesma ordem de lock sempre

	// 4. Bloqueia as contas envolvidas com SELECT ... FOR UPDATE
	accounts, err := r.lockAccountsTx(ctx, dbTx, accountIDs)
	if err != nil {
		return nil, false, err
	}

	// Verifica se todas as contas solicitadas existem no banco
	for _, id := range accountIDs {
		if _, ok := accounts[id]; !ok {
			return nil, false, fmt.Errorf("%w: conta ID %s não existe", domain.ErrAccountNotFound, id)
		}
	}

	// Validação de consistência cambial: todas as contas na transação devem ter a mesma moeda
	baseCurrency := ""
	for _, acc := range accounts {
		if baseCurrency == "" {
			baseCurrency = acc.Currency
		} else if baseCurrency != acc.Currency {
			return nil, false, fmt.Errorf("incompatibilidade de moeda contábil: transação contém moedas %s e %s", baseCurrency, acc.Currency)
		}
	}

	// 5. Validação de saldo suficiente: calcula os totais já existentes para as contas travadas
	currentBalances, err := r.calculateAccountsBalancesTx(ctx, dbTx, accountIDs)
	if err != nil {
		return nil, false, err
	}

	// Simula a aplicação dos novos lançamentos para verificar se alguma conta fica negativa
	projectedDebits := make(map[string]int64)
	projectedCredits := make(map[string]int64)
	for id, bal := range currentBalances {
		projectedDebits[id] = bal.totalDebit
		projectedCredits[id] = bal.totalCredit
	}

	for _, entry := range tx.Entries {
		if entry.Type == domain.EntryTypeDebit {
			projectedDebits[entry.AccountID] += entry.Amount
		} else if entry.Type == domain.EntryTypeCredit {
			projectedCredits[entry.AccountID] += entry.Amount
		}
	}

	// Verifica o saldo projetado para cada conta
	for _, id := range accountIDs {
		acc := accounts[id]
		newBal := domain.CalculateBalance(acc.Type, projectedDebits[id], projectedCredits[id])
		if newBal < 0 {
			curBal := domain.CalculateBalance(acc.Type, currentBalances[id].totalDebit, currentBalances[id].totalCredit)
			return nil, false, fmt.Errorf("%w: a conta '%s' (%s) possui saldo atual de %s e o lançamento resultaria em saldo negativo de %s",
				domain.ErrInsufficientBalance,
				acc.Name,
				acc.ID,
				domain.FormatMoney(curBal, acc.Currency),
				domain.FormatMoney(newBal, acc.Currency),
			)
		}
	}

	// 6. Insere a transação contábil no PostgreSQL
	if tx.ID == "" {
		tx.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	if tx.PostedAt.IsZero() {
		tx.PostedAt = now
	}
	if tx.CreatedAt.IsZero() {
		tx.CreatedAt = now
	}

	insertTxQuery := `
		INSERT INTO transactions (id, reference, description, posted_at, created_at)
		VALUES ($1, $2, $3, $4, $5);
	`
	var refParam *string
	if tx.Reference != "" {
		refParam = &tx.Reference
	}

	_, err = dbTx.ExecContext(ctx, insertTxQuery, tx.ID, refParam, tx.Description, tx.PostedAt, tx.CreatedAt)
	if err != nil {
		// Se houver colisão de chave de concorrência com o mesmo reference, tenta retornar o já salvo
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" && tx.Reference != "" {
			existing, fetchErr := r.findTransactionByReferenceTx(ctx, dbTx, tx.Reference)
			if fetchErr == nil && existing != nil {
				return existing, true, nil
			}
		}
		return nil, false, fmt.Errorf("falha ao inserir transação contábil: %w", err)
	}

	// 7. Insere os lançamentos (Entries) imutáveis vinculados à transação
	insertEntryQuery := `
		INSERT INTO entries (id, transaction_id, account_id, amount, type, created_at)
		VALUES ($1, $2, $3, $4, $5, $6);
	`
	for i := range tx.Entries {
		if tx.Entries[i].ID == "" {
			tx.Entries[i].ID = uuid.New().String()
		}
		tx.Entries[i].TransactionID = tx.ID
		if tx.Entries[i].CreatedAt.IsZero() {
			tx.Entries[i].CreatedAt = now
		}

		_, err = dbTx.ExecContext(ctx, insertEntryQuery,
			tx.Entries[i].ID,
			tx.Entries[i].TransactionID,
			tx.Entries[i].AccountID,
			tx.Entries[i].Amount,
			tx.Entries[i].Type,
			tx.Entries[i].CreatedAt,
		)
		if err != nil {
			return nil, false, fmt.Errorf("falha ao inserir perna contábil %d: %w", i+1, err)
		}
	}

	// 8. Confirmação atômica no banco (COMMIT)
	if err := dbTx.Commit(); err != nil {
		return nil, false, fmt.Errorf("falha ao efetivar (commit) transação contábil: %w", err)
	}

	return tx, false, nil
}

type balanceAccumulator struct {
	totalDebit  int64
	totalCredit int64
}

// lockAccountsTx bloqueia com FOR UPDATE e retorna o mapa de contas.
func (r *Repository) lockAccountsTx(ctx context.Context, tx *sql.Tx, accountIDs []string) (map[string]*domain.Account, error) {
	query := `
		SELECT id, name, type, currency, created_at 
		FROM accounts 
		WHERE id = ANY($1) 
		ORDER BY id 
		FOR UPDATE;
	`
	rows, err := tx.QueryContext(ctx, query, pq.Array(accountIDs))
	if err != nil {
		return nil, fmt.Errorf("falha ao bloquear contas com FOR UPDATE: %w", err)
	}
	defer rows.Close()

	accounts := make(map[string]*domain.Account)
	for rows.Next() {
		var acc domain.Account
		var accType string
		if err := rows.Scan(&acc.ID, &acc.Name, &accType, &acc.Currency, &acc.CreatedAt); err != nil {
			return nil, fmt.Errorf("falha ao ler conta bloqueada: %w", err)
		}
		acc.Type = domain.AccountType(accType)
		accounts[acc.ID] = &acc
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return accounts, nil
}

// calculateAccountsBalancesTx calcula a soma de débitos e créditos atuais para as contas selecionadas.
func (r *Repository) calculateAccountsBalancesTx(ctx context.Context, tx *sql.Tx, accountIDs []string) (map[string]balanceAccumulator, error) {
	balances := make(map[string]balanceAccumulator)
	for _, id := range accountIDs {
		balances[id] = balanceAccumulator{}
	}

	query := `
		SELECT 
			account_id,
			COALESCE(SUM(CASE WHEN type = 'DEBIT' THEN amount ELSE 0 END), 0) AS total_debit,
			COALESCE(SUM(CASE WHEN type = 'CREDIT' THEN amount ELSE 0 END), 0) AS total_credit
		FROM entries
		WHERE account_id = ANY($1)
		GROUP BY account_id;
	`
	rows, err := tx.QueryContext(ctx, query, pq.Array(accountIDs))
	if err != nil {
		return nil, fmt.Errorf("falha ao consultar saldos acumulados: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var accID string
		var deb, cred int64
		if err := rows.Scan(&accID, &deb, &cred); err != nil {
			return nil, fmt.Errorf("falha ao ler saldo acumulado: %w", err)
		}
		balances[accID] = balanceAccumulator{
			totalDebit:  deb,
			totalCredit: cred,
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return balances, nil
}

// findTransactionByReferenceTx busca uma transação já existente pela referência de idempotência.
func (r *Repository) findTransactionByReferenceTx(ctx context.Context, tx *sql.Tx, reference string) (*domain.Transaction, error) {
	query := `SELECT id, reference, description, posted_at, created_at FROM transactions WHERE reference = $1;`
	row := tx.QueryRowContext(ctx, query, reference)

	var t domain.Transaction
	var refVal sql.NullString
	if err := row.Scan(&t.ID, &refVal, &t.Description, &t.PostedAt, &t.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("falha ao consultar transação existente por referência: %w", err)
	}
	if refVal.Valid {
		t.Reference = refVal.String
	}

	// Busca os lançamentos vinculados
	entriesQuery := `
		SELECT id, transaction_id, account_id, amount, type, created_at
		FROM entries
		WHERE transaction_id = $1
		ORDER BY created_at ASC;
	`
	rows, err := tx.QueryContext(ctx, entriesQuery, t.ID)
	if err != nil {
		return nil, fmt.Errorf("falha ao buscar lançamentos da transação idempotente: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var e domain.Entry
		var entryType string
		if err := rows.Scan(&e.ID, &e.TransactionID, &e.AccountID, &e.Amount, &entryType, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("falha ao ler lançamento: %w", err)
		}
		e.Type = domain.EntryType(entryType)
		t.Entries = append(t.Entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return &t, nil
}

