-- ==============================================================================
-- Schema Inicial: Ledger Financeiro por Partidas Dobradas & Motor de Conciliação
-- ==============================================================================

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- 1. Tabela de Contas Contábeis (Plano de Contas)
CREATE TABLE IF NOT EXISTS accounts (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(255) NOT NULL,
    type VARCHAR(20) NOT NULL CHECK (type IN ('ASSET', 'LIABILITY', 'EQUITY', 'REVENUE', 'EXPENSE')),
    currency VARCHAR(3) NOT NULL DEFAULT 'BRL',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 2. Tabela de Transações (Agrupador Atômico)
CREATE TABLE IF NOT EXISTS transactions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    reference VARCHAR(255) UNIQUE, -- Chave de idempotência ou ID de origem externa
    description TEXT NOT NULL,
    posted_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 3. Tabela de Lançamentos Contábeis (Entries / Postings / Pernas contábeis)
-- Cada lançamento é imutável (append-only) para garantir auditoria financeira completa.
CREATE TABLE IF NOT EXISTS entries (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    transaction_id UUID NOT NULL REFERENCES transactions(id) ON DELETE RESTRICT,
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    amount BIGINT NOT NULL CHECK (amount > 0), -- Sempre em centavos (ex: 1000 = R$ 10,00)
    type VARCHAR(10) NOT NULL CHECK (type IN ('DEBIT', 'CREDIT')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_entries_transaction_id ON entries(transaction_id);
CREATE INDEX idx_entries_account_id ON entries(account_id);

-- 4. Extrato Bancário / Adquirente Externo (Para o Motor de Conciliação)
CREATE TABLE IF NOT EXISTS external_statement_items (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    provider VARCHAR(50) NOT NULL, -- Ex: 'ITAU', 'CIELO', 'STRIPE'
    external_id VARCHAR(255) NOT NULL,
    amount BIGINT NOT NULL CHECK (amount > 0),
    type VARCHAR(10) NOT NULL CHECK (type IN ('DEBIT', 'CREDIT')),
    transaction_date TIMESTAMPTZ NOT NULL,
    description TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_provider_external_id UNIQUE (provider, external_id)
);

-- 5. Batimentos da Conciliação
CREATE TABLE IF NOT EXISTS reconciliation_matches (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    statement_item_id UUID NOT NULL REFERENCES external_statement_items(id),
    ledger_entry_id UUID NOT NULL REFERENCES entries(id),
    status VARCHAR(20) NOT NULL CHECK (status IN ('MATCHED', 'DISCREPANT', 'MANUAL_REVIEW')),
    discrepancy_reason TEXT,
    matched_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

