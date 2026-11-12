package domain

import (
	"time"
)

// ReconciliationStatus indica o estado de conferência entre o extrato externo e o ledger interno.
type ReconciliationStatus string

const (
	StatusPending      ReconciliationStatus = "PENDING"       // Aguardando batimento
	StatusMatched      ReconciliationStatus = "MATCHED"       // Conciliado com sucesso
	StatusDiscrepant   ReconciliationStatus = "DISCREPANT"    // Divergência encontrada (valor ou data)
	StatusManualReview ReconciliationStatus = "MANUAL_REVIEW" // Requer intervenção humana
)

// ExternalStatementItem representa um item de extrato vindo de um banco ou adquirente (ex: OFX, CNAB, webhook).
type ExternalStatementItem struct {
	ID          string    `json:"id"`
	Provider    string    `json:"provider"`  // Ex: "ITAU", "BRADESCO", "STRIPE"
	ExternalID  string    `json:"external_id"`
	Amount      int64     `json:"amount"`    // Em centavos
	Type        EntryType `json:"type"`      // DEBIT ou CREDIT
	Date        time.Time `json:"date"`
	Description string    `json:"description"`
}

// ReconciliationMatch associa o registro bancário externo a uma transação interna do ledger.
type ReconciliationMatch struct {
	ID                string               `json:"id"`
	StatementItemID   string               `json:"statement_item_id"`
	LedgerEntryID     string               `json:"ledger_entry_id"`
	Status            ReconciliationStatus `json:"status"`
	DiscrepancyReason string               `json:"discrepancy_reason,omitempty"`
	MatchedAt         time.Time            `json:"matched_at"`
}

