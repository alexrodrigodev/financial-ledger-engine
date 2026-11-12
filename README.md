# Financial Ledger Engine (Motor Contábil por Partidas Dobradas)

Um motor financeiro de liquidação e razão contábil (Double-Entry Bookkeeping Engine) construído em Go (Golang) de alta performance e resiliência, projetado para operar com consistência estrita (ACID) sobre PostgreSQL.

---

## Arquitetura e Princípios de Domínio

### 1. Princípio das Partidas Dobradas (Double-Entry Bookkeeping)
- Estrutura Atômica: Toda transação contábil agrupa no mínimo duas pernas contábeis (Entries): um débito e um crédito.
- Equilíbrio Contábil Estrito: A soma de todos os lançamentos a débito deve ser exatamente igual à soma de todos os lançamentos a crédito (Débitos = Créditos). Lançamentos desbalanceados são rejeitados com código HTTP 422 Unprocessable Entity.
- Integridade Monetária: Proibição explícita do uso de tipos de ponto flutuante (float64) para cálculos monetários. Todos os valores são manipulados em inteiros de 64 bits (int64), representando a menor fração da moeda (centavos: R$ 10,00 = 1000).

### 2. Natureza Contábil das Contas e Cálculo de Saldo
O motor classifica as contas de acordo com a sua natureza contábil oficial:
- Ativo (ASSET) e Despesa (EXPENSE): Natureza Devedora. Lançamentos a Débito aumentam (+) o saldo; lançamentos a Crédito reduzem (-) o saldo. (Saldo = Débito - Crédito).
- Passivo (LIABILITY), Patrimônio Líquido (EQUITY) e Receita (REVENUE): Natureza Credora. Lançamentos a Crédito aumentam (+) o saldo; lançamentos a Débito reduzem (-) o saldo. (Saldo = Crédito - Débito).

### 3. Proteção Contra Saldo Insuficiente (Overdraft Protection)
Antes de efetivar qualquer lançamento, o motor calcula o saldo projetado resultante de cada conta afetada. Se a operação resultar em saldo negativo, a transação inteira é abortada com ROLLBACK e retorna erro 422 Unprocessable Entity.

### 4. Idempotência e Concorrência Segura no PostgreSQL
- Idempotency-Key no Header HTTP: Todo envio de transação exige a chave de idempotência no cabeçalho Idempotency-Key (ou X-Idempotency-Key).
- Garantia de Não-Duplicação: Re-execuções com a mesma chave retornam o resultado da transação já gravada com código HTTP 200 OK e cabeçalho Idempotent-Replay: true.
- Prevenção de Deadlocks: As contas envolvidas em uma transação são bloqueadas via SELECT ... FOR UPDATE ordenadas lexicograficamente pelos seus IDs (ORDER BY id). Isso impede ciclos de bloqueio mútuo sob alta concorrência.
- Atomicidade Completa (ACID): Todas as operações são executadas dentro de uma transação PostgreSQL explícita (BEGIN, COMMIT, ROLLBACK).

---

## Como Executar

### Pré-requisitos
- Go 1.23+ instalado na máquina
- Banco de dados PostgreSQL ativo localmente ou na nuvem (ex: Neon.tech)
- (Opcional) Docker e Docker Compose

---

### Opção A: Execução Local com Go CLI (Recomendado / Nuvem Neon)

1. Configurar variáveis de ambiente:
Copie o modelo de exemplo para criar o .env:
PowerShell:
Copy-Item .env.example .env

Bash:
cp .env.example .env

Edite o arquivo .env com a sua DATABASE_URL do Neon:
PORT=8080
APP_ENV=development
DATABASE_URL=postgresql://usuario:senha@host:5432/neondb?sslmode=require

2. Iniciar o motor em Go:
go run cmd/api/main.go

3. Executar os testes automatizados (em outro terminal):
PowerShell:
.\scripts\test_api.ps1

---

### Opção B: Execução via Docker Compose

Suba o container do PostgreSQL e da API em um único comando:
docker compose up --build -d

Para acompanhar os logs da aplicação:
docker compose logs -f api

Para encerrar os serviços e limpar os volumes:
docker compose down -v

---

## Testes Automatizados

### 1. Testes Unitários de Domínio em Go
Os testes unitários cobrem validação de partidas dobradas, desbalanceamento, valores negativos, cálculo de saldo por natureza contábil e handlers HTTP:
go test -v ./...

### 2. Script de Testes de Integração Ponta a Ponta (Windows PowerShell)
Com o servidor rodando em um terminal, abra outro terminal e execute:
.\scripts\test_api.ps1

### 3. Script de Testes de Integração (Linux / macOS / WSL)
chmod +x scripts/test_api.sh
./scripts/test_api.sh

O script de integração valida automaticamente:
1. Healthcheck do motor (/healthz)
2. Criação de contas contábeis (Ativo, Patrimônio Líquido e Despesas)
3. Aporte inicial balanceado de R$ 1.000,00
4. Replay idempotente com a mesma chave (evitando duplicidade)
5. Rejeição de transação desbalanceada (Débito != Crédito)
6. Rejeição de transação por saldo insuficiente (Overdraft)
7. Execução de débito válido de R$ 350,00 e conferência matemática dos saldos finais

---

## Referência da API HTTP

### 1. Healthcheck
Endpoint: GET /healthz
curl -X GET http://localhost:8080/healthz

Resposta esperada (200 OK):
{
  "service": "financial-ledger-engine",
  "status": "healthy",
  "timestamp": "2026-11-11T20:00:00Z"
}

---

### 2. Criar Conta Contábil
Endpoint: POST /api/v1/accounts
Tipos permitidos: ASSET, LIABILITY, EQUITY, REVENUE, EXPENSE.

curl -X POST http://localhost:8080/api/v1/accounts -H "Content-Type: application/json" -d "{\"name\": \"Banco Conta Corrente\", \"type\": \"ASSET\", \"currency\": \"BRL\"}"

Resposta esperada (201 Created):
{
  "id": "e67414b5-6f68-45a8-ba50-13f6e1f0ce14",
  "name": "Banco Conta Corrente",
  "type": "ASSET",
  "currency": "BRL",
  "created_at": "2026-11-11T20:00:00Z"
}

---

### 3. Executar Transação Contábil
Endpoint: POST /api/v1/transactions
Header Obrigatório: Idempotency-Key: <chave-unica>

curl -X POST http://localhost:8080/api/v1/transactions -H "Content-Type: application/json" -H "Idempotency-Key: aporte-capital-001" -d "{\"description\": \"Aporte inicial de capital\", \"entries\": [{\"account_id\": \"e67414b5-6f68-45a8-ba50-13f6e1f0ce14\", \"amount\": 100000, \"type\": \"DEBIT\"}, {\"account_id\": \"87b0f6df-b118-4235-85db-385d3810f1ce\", \"amount\": 100000, \"type\": \"CREDIT\"}]}"

Resposta esperada (201 Created):
{
  "id": "f86b1c31-9a7c-4712-ba29-0ec82813589b",
  "reference": "aporte-capital-001",
  "description": "Aporte inicial de capital",
  "posted_at": "2026-11-11T20:00:00Z"
}

---

### 4. Consultar Saldo Consolidado da Conta
Endpoint: GET /api/v1/accounts/{id}/balance
curl -X GET http://localhost:8080/api/v1/accounts/e67414b5-6f68-45a8-ba50-13f6e1f0ce14/balance

Resposta esperada (200 OK):
{
  "account_id": "e67414b5-6f68-45a8-ba50-13f6e1f0ce14",
  "account_name": "Banco Conta Corrente",
  "account_type": "ASSET",
  "currency": "BRL",
  "balance": 100000,
  "balance_formatted": "BRL 1000.00",
  "total_debit": 100000,
  "total_credit": 0,
  "updated_at": "2026-11-11T20:00:00Z"
}

---

## Estrutura do Projeto

financial-ledger-engine/
├── cmd/
│   └── api/
│       └── main.go                         # Ponto de entrada e boot do servidor
├── internal/
│   ├── config/
│   │   └── config.go                       # Leitura das variáveis de ambiente
│   ├── domain/
│   │   ├── ledger.go                       # Modelos contábeis e regras de negócio
│   │   ├── ledger_test.go                  # Testes unitários de domínio
│   │   └── reconciliation.go               # Tipos do motor de conciliação
│   ├── repository/
│   │   └── postgres/
│   │       └── repository.go               # Camada de persistência SQL com transações ACID
│   └── transport/
│       └── http/
│           ├── handler.go                  # Controladores REST HTTP
│           ├── handler_test.go             # Testes unitários dos handlers
│           └── router.go                   # Roteamento Chi e middlewares
├── migrations/
│   └── 000001_init_ledger_schema.sql       # DDL inicial do banco de dados
├── scripts/
│   ├── test_api.sh                         # Testes automatizados via Bash
│   └── test_api.ps1                        # Testes automatizados via PowerShell
├── docker-compose.yml                      # Orquestração local de containers
├── Dockerfile                              # Multi-stage build estático em Go
├── go.mod                                  # Módulo Go e dependências
├── Makefile                                # Atalhos de compilação e testes
└── README.md                               # Documentação oficial do projeto