# Financial Ledger Engine (Motor Contábil por Partidas Dobradas)

Um motor financeiro de liquidação e razão contábil (**Double-Entry Bookkeeping Engine**) construído em **Go (Golang)** de alta performance e resiliência, projetado para operar com consistência estrita (**ACID**) sobre **PostgreSQL**.

---

## 🎯 Arquitetura e Princípios de Domínio

### 1. Princípio das Partidas Dobradas (Double-Entry Bookkeeping)
- Toda transação contábil agrupa no mínimo **duas pernas contábeis (Entries)**: um débito e um crédito.
- **Equilíbrio Contábil Estrito:** A soma de todos os lançamentos a débito deve ser exatamente igual à soma de todos os lançamentos a crédito ($\sum \text{Débitos} = \sum \text{Créditos}$). Lançamentos desbalanceados são rejeitados com código HTTP `422 Unprocessable Entity`.
- **Integridade Monetária:** Proibição explícita do uso de tipos de ponto flutuante (`float64`) para cálculos financeiros. Todos os valores são manipulados em números inteiros de 64 bits (`int64`), representando a menor fração da moeda (centavos: R$ 10,00 = `1000`).

### 2. Natureza Contábil das Contas e Cálculo de Saldo
O motor classifica as contas contábeis de acordo com a sua natureza oficial:
- **Ativo (`ASSET`)** e **Despesa (`EXPENSE`)**: Natureza Devedora. Lançamentos a **Débito aumentam (+)** o saldo, lançamentos a **Crédito reduzem (-)** o saldo. Saldo = $\text{Débito} - \text{Crédito}$.
- **Passivo (`LIABILITY`)**, **Patrimônio Líquido (`EQUITY`)** e **Receita (`REVENUE`)**: Natureza Credora. Lançamentos a **Crédito aumentam (+)** o saldo, lançamentos a **Débito reduzem (-)** o saldo. Saldo = $\text{Crédito} - \text{Débito}$.

### 3. Proteção Contra Saldo Insuficiente (Overdraft Protection)
Antes de efetivar qualquer lançamento, o motor calcula o saldo projetado resultante de cada conta afetada. Se a operação resultar em saldo negativo, a transação inteira é abortada com `ROLLBACK` e retorna erro `422 Unprocessable Entity`.

### 4. Idempotência e Concorrência Segura no PostgreSQL
- **Idempotency-Key no Header HTTP:** Todo envio de transação exige a chave de idempotência no cabeçalho `Idempotency-Key` (ou `X-Idempotency-Key`).
- **Garantia de Não-Duplicação:** Re-execuções com a mesma chave retornam o resultado da transação já gravada com código HTTP `200 OK` e cabeçalho `Idempotent-Replay: true`.
- **Prevenção de Deadlocks:** As contas envolvidas em uma transação são bloqueadas via `SELECT ... FOR UPDATE` ordenadas lexicograficamente pelos seus IDs (`ORDER BY id`). Isso impede ciclos de bloqueio mútuo sob alta concorrência.
- **Atomicidade Completa (ACID):** Todas as operações são executadas dentro de uma transação PostgreSQL explícita (`BEGIN`, `COMMIT`, `ROLLBACK`).

---

## 🚀 Como Executar

### Pré-requisitos
- [Docker](https://www.docker.com/) e [Docker Compose](https://docs.docker.com/compose/) instalados (ou Go 1.23+ e PostgreSQL 16+ caso execute localmente).

### Execução via Docker Compose (Recomendado)

Suba o container do PostgreSQL e da API em um único comando:

```bash
docker compose up --build -d
```

Para acompanhar os logs da aplicação:
```bash
docker compose logs -f api
```

Para encerrar os serviços e limpar os volumes:
```bash
docker compose down -v
```

---

## 🧪 Testes Automatizados

### 1. Testes Unitários em Go
Os testes unitários cobrem validação de partidas dobradas, desbalanceamento, valores negativos, cálculo de saldo por natureza contábil e handlers HTTP:

```bash
go test -v ./...
```

### 2. Script de Testes de Integração via cURL (Linux / macOS / WSL / Git Bash)

```bash
chmod +x scripts/test_api.sh
./scripts/test_api.sh
```

### 3. Script de Testes via PowerShell (Windows)

```powershell
.\scripts\test_api.ps1
```

O script executa um fluxo completo:
1. Verifica o endpoint `/healthz`
2. Cria contas de Ativo (Banco), Patrimônio Líquido (Capital Social) e Despesa (Cloud)
3. Executa aporte inicial balanceado de R$ 1.000,00
4. Testa replay idempotente com a mesma chave
5. Testa rejeição de transação desbalanceada
6. Testa rejeição de transação por saldo insuficiente
7. Executa pagamento válido de R$ 350,00 e consulta os saldos finais

---

## 📡 Referência da API HTTP

### 1. Healthcheck
- **GET** `/healthz`

```bash
curl -X GET http://localhost:8080/healthz
```

**Resposta (200 OK):**
```json
{
  "service": "financial-ledger-engine",
  "status": "healthy",
  "timestamp": "2026-11-11T20:00:00Z"
}
```

---

### 2. Criar Conta Contábil
- **POST** `/api/v1/accounts`

**Payload:**
```json
{
  "name": "Banco Conta Corrente",
  "type": "ASSET",
  "currency": "BRL"
}
```

Tipos permitidos: `ASSET`, `LIABILITY`, `EQUITY`, `REVENUE`, `EXPENSE`.

```bash
curl -X POST http://localhost:8080/api/v1/accounts \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Banco Conta Corrente",
    "type": "ASSET",
    "currency": "BRL"
  }'
```

**Resposta (201 Created):**
```json
{
  "id": "e67414b5-6f68-45a8-ba50-13f6e1f0ce14",
  "name": "Banco Conta Corrente",
  "type": "ASSET",
  "currency": "BRL",
  "created_at": "2026-11-11T20:00:00Z"
}
```

---

### 3. Executar Transação Contábil
- **POST** `/api/v1/transactions`
- **Header Obrigatório:** `Idempotency-Key: <chave-unica>`

**Exemplo de Aporte Inicial (Débito no Banco, Crédito no Capital Social):**
```bash
curl -X POST http://localhost:8080/api/v1/transactions \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: aporte-capital-001" \
  -d '{
    "description": "Aporte inicial de capital",
    "entries": [
      {
        "account_id": "e67414b5-6f68-45a8-ba50-13f6e1f0ce14",
        "amount": 100000,
        "type": "DEBIT"
      },
      {
        "account_id": "87b0f6df-b118-4235-85db-385d3810f1ce",
        "amount": 100000,
        "type": "CREDIT"
      }
    ]
  }'
```

**Resposta (201 Created):**
```json
{
  "id": "f86b1c31-9a7c-4712-ba29-0ec82813589b",
  "reference": "aporte-capital-001",
  "description": "Aporte inicial de capital",
  "entries": [
    {
      "id": "55c4d2d4-bb3f-42a9-b31c-6d808c48a7eb",
      "transaction_id": "f86b1c31-9a7c-4712-ba29-0ec82813589b",
      "account_id": "e67414b5-6f68-45a8-ba50-13f6e1f0ce14",
      "amount": 100000,
      "type": "DEBIT",
      "created_at": "2026-11-11T20:00:00Z"
    },
    {
      "id": "76e31b67-93be-462a-89bc-980b3a3d3c87",
      "transaction_id": "f86b1c31-9a7c-4712-ba29-0ec82813589b",
      "account_id": "87b0f6df-b118-4235-85db-385d3810f1ce",
      "amount": 100000,
      "type": "CREDIT",
      "created_at": "2026-11-11T20:00:00Z"
    }
  ],
  "posted_at": "2026-11-11T20:00:00Z",
  "created_at": "2026-11-11T20:00:00Z"
}
```

---

### 4. Consultar Saldo Consolidado da Conta
- **GET** `/api/v1/accounts/{id}/balance`

```bash
curl -X GET http://localhost:8080/api/v1/accounts/e67414b5-6f68-45a8-ba50-13f6e1f0ce14/balance
```

**Resposta (200 OK):**
```json
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
```

---

## 🗄️ Estrutura do Projeto

```
financial-ledger-engine/
├── cmd/
│   └── api/
│       └── main.go                         # Ponto de entrada, boot com retry e graceful shutdown
├── internal/
│   ├── config/
│   │   └── config.go                       # Carregamento de variáveis de ambiente
│   ├── domain/
│   │   ├── ledger.go                       # Modelos contábeis, regras de partidas dobradas e saldos
│   │   ├── ledger_test.go                  # Testes unitários de domínio
│   │   └── reconciliation.go               # Tipos do motor de conciliação
│   ├── repository/
│   │   └── postgres/
│   │       └── repository.go               # Persistência PostgreSQL com FOR UPDATE e transações ACID
│   └── transport/
│       └── http/
│           ├── handler.go                  # Controladores HTTP REST
│           ├── handler_test.go             # Testes unitários dos endpoints HTTP
│           └── router.go                   # Roteamento Chi e middlewares
├── migrations/
│   └── 000001_init_ledger_schema.sql       # DDL inicial do banco de dados
├── scripts/
│   ├── test_api.sh                         # Script de testes automatizados cURL (Bash)
│   └── test_api.ps1                        # Script de testes automatizados (PowerShell)
├── docker-compose.yml                      # Orquestração do PostgreSQL e da API
├── Dockerfile                              # Multi-stage build Go estático (~15MB)
├── go.mod                                  # Módulo Go financial-ledger-engine
├── Makefile                                # Comandos úteis de build, teste e execução
└── README.md                               # Documentação completa
```

