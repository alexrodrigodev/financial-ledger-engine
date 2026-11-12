package domain_test

import (
	"testing"
	"time"

	"financial-ledger-engine/internal/domain"
)

func TestTransaction_Validate(t *testing.T) {
	tests := []struct {
		name        string
		transaction domain.Transaction
		wantErr     error
	}{
		{
			name: "Transação balanceada válida (R$ 100,00)",
			transaction: domain.Transaction{
				ID:          "tx-1",
				Description: "Transferência bancária",
				PostedAt:    time.Now(),
				Entries: []domain.Entry{
					{AccountID: "acc-bank", Amount: 10000, Type: domain.EntryTypeDebit},
					{AccountID: "acc-customer", Amount: 10000, Type: domain.EntryTypeCredit},
				},
			},
			wantErr: nil,
		},
		{
			name: "Transação válida com 3 pernas contábeis (split payment: 1 débito 100, 2 créditos 60 e 40)",
			transaction: domain.Transaction{
				ID:          "tx-split",
				Description: "Pagamento com taxa dividida",
				Entries: []domain.Entry{
					{AccountID: "acc-payable", Amount: 10000, Type: domain.EntryTypeDebit},
					{AccountID: "acc-bank", Amount: 6000, Type: domain.EntryTypeCredit},
					{AccountID: "acc-discount", Amount: 4000, Type: domain.EntryTypeCredit},
				},
			},
			wantErr: nil,
		},
		{
			name: "Transação com menos de duas pernas contábeis",
			transaction: domain.Transaction{
				ID:          "tx-2",
				Description: "Lançamento incompleto",
				Entries: []domain.Entry{
					{AccountID: "acc-bank", Amount: 10000, Type: domain.EntryTypeDebit},
				},
			},
			wantErr: domain.ErrInsufficientEntries,
		},
		{
			name: "Transação desbalanceada (Débito diferente de Crédito)",
			transaction: domain.Transaction{
				ID:          "tx-3",
				Description: "Débito 100 e Crédito 80",
				Entries: []domain.Entry{
					{AccountID: "acc-bank", Amount: 10000, Type: domain.EntryTypeDebit},
					{AccountID: "acc-customer", Amount: 8000, Type: domain.EntryTypeCredit},
				},
			},
			wantErr: domain.ErrUnbalancedTransaction,
		},
		{
			name: "Lançamento com valor zero",
			transaction: domain.Transaction{
				ID:          "tx-4",
				Description: "Valor zero",
				Entries: []domain.Entry{
					{AccountID: "acc-bank", Amount: 0, Type: domain.EntryTypeDebit},
					{AccountID: "acc-customer", Amount: 0, Type: domain.EntryTypeCredit},
				},
			},
			wantErr: domain.ErrInvalidAmount,
		},
		{
			name: "Lançamento com valor negativo",
			transaction: domain.Transaction{
				ID:          "tx-5",
				Description: "Valor negativo",
				Entries: []domain.Entry{
					{AccountID: "acc-bank", Amount: -500, Type: domain.EntryTypeDebit},
					{AccountID: "acc-customer", Amount: -500, Type: domain.EntryTypeCredit},
				},
			},
			wantErr: domain.ErrInvalidAmount,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.transaction.Validate()
			if err != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCalculateBalance(t *testing.T) {
	tests := []struct {
		name        string
		accType     domain.AccountType
		totalDebit  int64
		totalCredit int64
		wantBalance int64
	}{
		{
			name:        "Conta de Ativo (ASSET): Débito aumenta, Crédito diminui",
			accType:     domain.AccountTypeAsset,
			totalDebit:  15000,
			totalCredit: 5000,
			wantBalance: 10000,
		},
		{
			name:        "Conta de Despesa (EXPENSE): Débito aumenta, Crédito diminui",
			accType:     domain.AccountTypeExpense,
			totalDebit:  8000,
			totalCredit: 2000,
			wantBalance: 6000,
		},
		{
			name:        "Conta de Passivo (LIABILITY): Crédito aumenta, Débito diminui",
			accType:     domain.AccountTypeLiability,
			totalDebit:  3000,
			totalCredit: 10000,
			wantBalance: 7000,
		},
		{
			name:        "Conta de Patrimônio Líquido (EQUITY): Crédito aumenta, Débito diminui",
			accType:     domain.AccountTypeEquity,
			totalDebit:  0,
			totalCredit: 50000,
			wantBalance: 50000,
		},
		{
			name:        "Conta de Receita (REVENUE): Crédito aumenta, Débito diminui",
			accType:     domain.AccountTypeRevenue,
			totalDebit:  500,
			totalCredit: 15500,
			wantBalance: 15000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := domain.CalculateBalance(tt.accType, tt.totalDebit, tt.totalCredit)
			if got != tt.wantBalance {
				t.Errorf("CalculateBalance() = %d, want %d", got, tt.wantBalance)
			}
		})
	}
}

func TestFormatMoney(t *testing.T) {
	tests := []struct {
		cents    int64
		currency string
		expected string
	}{
		{1000, "BRL", "BRL 10.00"},
		{1050, "BRL", "BRL 10.50"},
		{5, "USD", "USD 0.05"},
		{-2500, "BRL", "-BRL 25.00"},
	}

	for _, tt := range tests {
		got := domain.FormatMoney(tt.cents, tt.currency)
		if got != tt.expected {
			t.Errorf("FormatMoney(%d, %s) = %s, want %s", tt.cents, tt.currency, got, tt.expected)
		}
	}
}

func TestAccount_Validate(t *testing.T) {
	validAcc := domain.Account{Name: "Caixa", Type: domain.AccountTypeAsset, Currency: "BRL"}
	if err := validAcc.Validate(); err != nil {
		t.Errorf("esperado conta válida, erro: %v", err)
	}

	emptyName := domain.Account{Name: "", Type: domain.AccountTypeAsset, Currency: "BRL"}
	if err := emptyName.Validate(); err != domain.ErrInvalidAccountName {
		t.Errorf("esperado ErrInvalidAccountName, erro: %v", err)
	}

	invalidType := domain.Account{Name: "Caixa", Type: "INVALID", Currency: "BRL"}
	if err := invalidType.Validate(); err != domain.ErrInvalidAccountType {
		t.Errorf("esperado ErrInvalidAccountType, erro: %v", err)
	}

	invalidCurrency := domain.Account{Name: "Caixa", Type: domain.AccountTypeAsset, Currency: "REAL"}
	if err := invalidCurrency.Validate(); err != domain.ErrInvalidCurrency {
		t.Errorf("esperado ErrInvalidCurrency, erro: %v", err)
	}
}
