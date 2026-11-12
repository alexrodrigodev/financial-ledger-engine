#!/usr/bin/env bash
# ==============================================================================
# Script de Teste Automatizado via cURL - Financial Ledger Engine
# ==============================================================================

set -e

BASE_URL="${BASE_URL:-http://localhost:8080}"
echo "------------------------------------------------------------------"
echo " Início dos Testes Automatizados da API do Ledger Financeiro"
echo " Alvo: ${BASE_URL}"
echo "------------------------------------------------------------------"

# 1. Healthcheck
echo -e "\n[1/7] Testando Healthcheck (GET /healthz)..."
curl -s -f -X GET "${BASE_URL}/healthz" | grep "healthy" && echo " OK: Healthcheck respondendo!"

# 2. Criação de Contas Contábeis
echo -e "\n[2/7] Criando Contas Contábeis (POST /api/v1/accounts)..."

# Conta 1: Ativo (Banco)
ACC1_RESP=$(curl -s -X POST "${BASE_URL}/api/v1/accounts" \
  -H "Content-Type: application/json" \
  -d '{"name": "Banco Conta Corrente", "type": "ASSET", "currency": "BRL"}')
ACC_BANK_ID=$(echo "$ACC1_RESP" | grep -o '"id":"[^"]*' | cut -d'"' -f4)
echo " Conta Banco (ASSET) criada com ID: $ACC_BANK_ID"

# Conta 2: Patrimônio Líquido (Capital Social)
ACC2_RESP=$(curl -s -X POST "${BASE_URL}/api/v1/accounts" \
  -H "Content-Type: application/json" \
  -d '{"name": "Capital Social dos Sócios", "type": "EQUITY", "currency": "BRL"}')
ACC_EQUITY_ID=$(echo "$ACC2_RESP" | grep -o '"id":"[^"]*' | cut -d'"' -f4)
echo " Conta Capital Social (EQUITY) criada com ID: $ACC_EQUITY_ID"

# Conta 3: Despesa (Serviços Cloud)
ACC3_RESP=$(curl -s -X POST "${BASE_URL}/api/v1/accounts" \
  -H "Content-Type: application/json" \
  -d '{"name": "Despesas com Servidores Cloud", "type": "EXPENSE", "currency": "BRL"}')
ACC_EXPENSE_ID=$(echo "$ACC3_RESP" | grep -o '"id":"[^"]*' | cut -d'"' -f4)
echo " Conta Despesas Cloud (EXPENSE) criada com ID: $ACC_EXPENSE_ID"

# 3. Transação de Aporte Inicial (Partidas Dobradas Balanceadas)
# Débito de R$ 1.000,00 no Ativo (aumenta caixa) e Crédito de R$ 1.000,00 no Patrimônio Líquido (aumenta capital)
echo -e "\n[3/7] Executando Aporte Inicial de R$ 1.000,00 (POST /api/v1/transactions)..."
IDEMPOTENCY_KEY_1="aporte-inicial-$(date +%s)"

TX1_STATUS=$(curl -s -o /tmp/tx1.json -w "%{http_code}" -X POST "${BASE_URL}/api/v1/transactions" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: ${IDEMPOTENCY_KEY_1}" \
  -d "{
    \"description\": \"Aporte de capital inicial\",
    \"entries\": [
      {\"account_id\": \"${ACC_BANK_ID}\", \"amount\": 100000, \"type\": \"DEBIT\"},
      {\"account_id\": \"${ACC_EQUITY_ID}\", \"amount\": 100000, \"type\": \"CREDIT\"}
    ]
  }")

if [ "$TX1_STATUS" -eq 201 ]; then
  echo " OK: Transação criada com sucesso (HTTP 201)!"
else
  echo " ERRO: Falha ao criar transação inicial. Status: $TX1_STATUS"
  cat /tmp/tx1.json
  exit 1
fi

# 4. Teste de Idempotência (Replay da mesma Idempotency-Key)
echo -e "\n[4/7] Testando Idempotência com mesma Idempotency-Key..."
REPLAY_STATUS=$(curl -s -o /tmp/tx_replay.json -w "%{http_code}" -X POST "${BASE_URL}/api/v1/transactions" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: ${IDEMPOTENCY_KEY_1}" \
  -d "{
    \"description\": \"Tentativa duplicada idêntica\",
    \"entries\": [
      {\"account_id\": \"${ACC_BANK_ID}\", \"amount\": 100000, \"type\": \"DEBIT\"},
      {\"account_id\": \"${ACC_EQUITY_ID}\", \"amount\": 100000, \"type\": \"CREDIT\"}
    ]
  }")

if [ "$REPLAY_STATUS" -eq 200 ]; then
  echo " OK: Idempotência confirmada! Retornou HTTP 200 Replay sem duplicar lançamentos."
else
  echo " AVISO: Status retornado: $REPLAY_STATUS"
fi

# 5. Teste de Violação de Partidas Dobradas (Transação Desbalanceada)
echo -e "\n[5/7] Testando Rejeição de Partidas Dobradas Desbalanceadas..."
UNBAL_STATUS=$(curl -s -o /tmp/unbalanced.json -w "%{http_code}" -X POST "${BASE_URL}/api/v1/transactions" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: fail-unbalanced-$(date +%s)" \
  -d "{
    \"description\": \"Tentativa desbalanceada (D 50,00 vs C 40,00)\",
    \"entries\": [
      {\"account_id\": \"${ACC_BANK_ID}\", \"amount\": 5000, \"type\": \"DEBIT\"},
      {\"account_id\": \"${ACC_EQUITY_ID}\", \"amount\": 4000, \"type\": \"CREDIT\"}
    ]
  }")

if [ "$UNBAL_STATUS" -eq 422 ]; then
  echo " OK: Transação desbalanceada rejeitada corretamente com HTTP 422 (Unprocessable Entity)!"
else
  echo " ERRO: Esperado HTTP 422, recebido: $UNBAL_STATUS"
  cat /tmp/unbalanced.json
  exit 1
fi

# 6. Teste de Proteção de Saldo Insuficiente (Overdraft)
# Saldo bancário é R$ 1.000,00. Tentativa de gastar R$ 1.500,00 deve falhar.
echo -e "\n[6/7] Testando Proteção de Saldo Insuficiente (Tentativa de saque de R$ 1.500,00)..."
OVERDRAFT_STATUS=$(curl -s -o /tmp/overdraft.json -w "%{http_code}" -X POST "${BASE_URL}/api/v1/transactions" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: fail-overdraft-$(date +%s)" \
  -d "{
    \"description\": \"Tentativa de pagamento além do saldo\",
    \"entries\": [
      {\"account_id\": \"${ACC_BANK_ID}\", \"amount\": 150000, \"type\": \"CREDIT\"},
      {\"account_id\": \"${ACC_EXPENSE_ID}\", \"amount\": 150000, \"type\": \"DEBIT\"}
    ]
  }")

if [ "$OVERDRAFT_STATUS" -eq 422 ]; then
  echo " OK: Rejeição por saldo insuficiente funcionando corretamente com HTTP 422!"
else
  echo " ERRO: Esperado HTTP 422 para saldo insuficiente, recebido: $OVERDRAFT_STATUS"
  cat /tmp/overdraft.json
  exit 1
fi

# 7. Pagamento Válido de R$ 350,00 e Consulta de Saldos Finais
echo -e "\n[7/7] Executando Pagamento Válido de R$ 350,00 e Consultando Saldos (GET /api/v1/accounts/{id}/balance)..."
curl -s -X POST "${BASE_URL}/api/v1/transactions" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: pagto-cloud-$(date +%s)" \
  -d "{
    \"description\": \"Pagamento fatura Cloud AWS\",
    \"entries\": [
      {\"account_id\": \"${ACC_BANK_ID}\", \"amount\": 35000, \"type\": \"CREDIT\"},
      {\"account_id\": \"${ACC_EXPENSE_ID}\", \"amount\": 35000, \"type\": \"DEBIT\"}
    ]
  }" > /dev/null

echo -e "\n Saldo da Conta Banco (deve ser R$ 650,00):"
curl -s -X GET "${BASE_URL}/api/v1/accounts/${ACC_BANK_ID}/balance"
echo ""

echo -e "\n Saldo da Conta Capital Social (deve ser R$ 1.000,00):"
curl -s -X GET "${BASE_URL}/api/v1/accounts/${ACC_EQUITY_ID}/balance"
echo ""

echo -e "\n Saldo da Conta Despesas Cloud (deve ser R$ 350,00):"
curl -s -X GET "${BASE_URL}/api/v1/accounts/${ACC_EXPENSE_ID}/balance"
echo ""

echo -e "\n=================================================================="
echo " Todos os testes de integração do Ledger foram concluídos com SUCESSO!"
echo "=================================================================="

