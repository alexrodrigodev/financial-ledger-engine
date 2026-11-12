package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Erros de domínio do Ledger
var (
	ErrUnbalancedTransaction = errors.New("a transação viola o princípio das partidas dobradas: a soma dos débitos deve ser igual à soma dos créditos")
	ErrInsufficientEntries   = errors.New("uma transação de partidas dobradas requer ao menos dois lançamentos (débito e crédito)")
	ErrInvalidAmount         = errors.New("o valor do lançamento deve ser estritamente positivo (maior que zero)")
	ErrInsufficientBalance   = errors.New("saldo insuficiente na conta para realizar a transação")
	ErrAccountNotFound       = errors.New("conta contábil não encontrada")
	ErrInvalidAccountType    = errors.New("tipo de conta contábil inválido: deve ser ASSET, LIABILITY, EQUITY, REVENUE ou EXPENSE")
	ErrInvalidCurrency       = errors.New("moeda inválida: deve conter 3 caracteres alfabéticos (ex: BRL, USD)")
	ErrMissingIdempotencyKey = errors.New("cabeçalho Idempotency-Key é obrigatório")
	ErrInvalidAccountName    = errors.New("o nome da conta não pode ser vazio")
)

// AccountType representa a classificação contábil da conta.
type AccountType string

const (
	AccountTypeAsset     AccountType = "ASSET"     // Ativo (ex: Caixa, Banco, Contas a Receber)
	AccountTypeLiability AccountType = "LIABILITY" // Passivo (ex: Fornecedores, Empréstimos)
	AccountTypeEquity    AccountType = "EQUITY"    // Patrimônio Líquido (ex: Capital Social)
	AccountTypeRevenue   AccountType = "REVENUE"   // Receita
	AccountTypeExpense   AccountType = "EXPENSE"   // Despesa
)

// IsValid valida se o tipo contábil pertence ao elenco oficial.
func (t AccountType) IsValid() bool {
	switch t {
	case AccountTypeAsset, AccountTypeLiability, AccountTypeEquity, AccountTypeRevenue, AccountTypeExpense:
		return true
	default:
		return false
	}
}

// IsNormalDebit indica se a conta possui natureza devedora (Ativos e Despesas).
// Para estas contas: Débito AUMENTA (+) o saldo, Crédito REDUZ (-) o saldo.
// Para contas de natureza credora (Passivos, Patrimônio Líquido e Receitas):
// Crédito AUMENTA (+) o saldo, Débito REDUZ (-) o saldo.
func (t AccountType) IsNormalDebit() bool {
	return t == AccountTypeAsset || t == AccountTypeExpense
}

// CalculateBalance calcula o saldo líquido da conta com base na sua natureza contábil.
func CalculateBalance(accType AccountType, totalDebit, totalCredit int64) int64 {
	if accType.IsNormalDebit() {
		return totalDebit - totalCredit
	}
	return totalCredit - totalDebit
}

// EntryType define a direção contábil do lançamento.
type EntryType string

const (
	EntryTypeDebit  EntryType = "DEBIT"  // Débito
	EntryTypeCredit EntryType = "CREDIT" // Crédito
)

// Account representa uma conta no plano de contas do Ledger.
type Account struct {
	ID        string      `json:"id"`
	Name      string      `json:"name"`
	Type      AccountType `json:"type"`
	Currency  string      `json:"currency"` // Ex: "BRL", "USD"
	CreatedAt time.Time   `json:"created_at"`
}

// Validate valida os dados da conta contábil.
func (a *Account) Validate() error {
	if strings.TrimSpace(a.Name) == "" {
		return ErrInvalidAccountName
	}
	if !a.Type.IsValid() {
		return ErrInvalidAccountType
	}
	if len(strings.TrimSpace(a.Currency)) != 3 {
		return ErrInvalidCurrency
	}
	a.Currency = strings.ToUpper(strings.TrimSpace(a.Currency))
	return nil
}

// Entry (ou Posting/Perna contábil) representa um lançamento individual em uma conta.
// REGRA FINANCEIRA CRÍTICA:
// Nunca utilize float64 para valores monetários devido a erros de arredondamento de ponto flutuante IEEE 754.
// Usamos int64 representando a menor fração da moeda (centavos: R$ 10,50 = 1050).
type Entry struct {
	ID            string    `json:"id"`
	TransactionID string    `json:"transaction_id"`
	AccountID     string    `json:"account_id"`
	Amount        int64     `json:"amount"` // Em centavos (ex: 1000 = R$ 10,00)
	Type          EntryType `json:"type"`   // DEBIT ou CREDIT
	CreatedAt     time.Time `json:"created_at,omitempty"`
}

// Transaction é um evento contábil atômico e imutável que agrupa múltiplos lançamentos.
type Transaction struct {
	ID          string    `json:"id"`
	Reference   string    `json:"reference"` // ID externo, chave de idempotência
	Description string    `json:"description"`
	Entries     []Entry   `json:"entries"`
	PostedAt    time.Time `json:"posted_at"`
	CreatedAt   time.Time `json:"created_at"`
}

// Validate verifica se a transação obedece às regras estritas de partidas dobradas.
func (t *Transaction) Validate() error {
	if len(t.Entries) < 2 {
		return ErrInsufficientEntries
	}

	var totalDebit int64
	var totalCredit int64

	for _, entry := range t.Entries {
		if strings.TrimSpace(entry.AccountID) == "" {
			return errors.New("o account_id de cada lançamento é obrigatório")
		}
		if entry.Amount <= 0 {
			return ErrInvalidAmount
		}

		switch entry.Type {
		case EntryTypeDebit:
			totalDebit += entry.Amount
		case EntryTypeCredit:
			totalCredit += entry.Amount
		default:
			return errors.New("tipo de lançamento inválido: deve ser DEBIT ou CREDIT")
		}
	}

	if totalDebit != totalCredit {
		return ErrUnbalancedTransaction
	}

	return nil
}

// AccountBalance consolida os totais e o saldo atual de uma conta.
type AccountBalance struct {
	AccountID        string      `json:"account_id"`
	AccountName      string      `json:"account_name"`
	AccountType      AccountType `json:"account_type"`
	Currency         string      `json:"currency"`
	Balance          int64       `json:"balance"` // Em centavos de acordo com a natureza da conta
	BalanceFormatted string      `json:"balance_formatted"`
	TotalDebit       int64       `json:"total_debit"`
	TotalCredit      int64       `json:"total_credit"`
	UpdatedAt        time.Time   `json:"updated_at"`
}

// FormatMoney formata centavos para apresentação monetária (ex: BRL 12.50).
func FormatMoney(cents int64, currency string) string {
	sign := ""
	val := cents
	if val < 0 {
		sign = "-"
		val = -val
	}
	major := val / 100
	minor := val % 100
	return fmt.Sprintf("%s%s %d.%02d", sign, currency, major, minor)
}
