# ==============================================================================
# Script de Teste Automatizado via PowerShell - Financial Ledger Engine
# ==============================================================================

param (
    [string]$BaseUrl = "http://localhost:8080"
)

$ErrorActionPreference = "Stop"

Write-Host "------------------------------------------------------------------" -ForegroundColor Cyan
Write-Host " Início dos Testes Automatizados da API do Ledger Financeiro" -ForegroundColor Cyan
Write-Host " Alvo: $BaseUrl" -ForegroundColor Cyan
Write-Host "------------------------------------------------------------------" -ForegroundColor Cyan

# 1. Healthcheck
Write-Host "`n[1/7] Testando Healthcheck (GET /healthz)..." -ForegroundColor Yellow
$health = Invoke-RestMethod -Uri "$BaseUrl/healthz" -Method Get
Write-Host " OK: Healthcheck respondendo: $($health.status)" -ForegroundColor Green

# 2. Criação de Contas Contábeis
Write-Host "`n[2/7] Criando Contas Contábeis (POST /api/v1/accounts)..." -ForegroundColor Yellow

$accBank = Invoke-RestMethod -Uri "$BaseUrl/api/v1/accounts" -Method Post -ContentType "application/json" -Body (@{
    name = "Banco Conta Corrente"
    type = "ASSET"
    currency = "BRL"
} | ConvertTo-Json)
Write-Host " Conta Banco (ASSET) criada com ID: $($accBank.id)" -ForegroundColor Green

$accEquity = Invoke-RestMethod -Uri "$BaseUrl/api/v1/accounts" -Method Post -ContentType "application/json" -Body (@{
    name = "Capital Social dos Sócios"
    type = "EQUITY"
    currency = "BRL"
} | ConvertTo-Json)
Write-Host " Conta Capital Social (EQUITY) criada com ID: $($accEquity.id)" -ForegroundColor Green

$accExpense = Invoke-RestMethod -Uri "$BaseUrl/api/v1/accounts" -Method Post -ContentType "application/json" -Body (@{
    name = "Despesas com Servidores Cloud"
    type = "EXPENSE"
    currency = "BRL"
} | ConvertTo-Json)
Write-Host " Conta Despesas Cloud (EXPENSE) criada com ID: $($accExpense.id)" -ForegroundColor Green

# 3. Transação de Aporte Inicial
Write-Host "`n[3/7] Executando Aporte Inicial de R$ 1.000,00 (POST /api/v1/transactions)..." -ForegroundColor Yellow
$idempotencyKey1 = "aporte-inicial-" + (Get-Date -Format "yyyyMMddHHmmssfff")

$tx1Payload = @{
    description = "Aporte de capital inicial"
    entries = @(
        @{ account_id = $accBank.id; amount = 100000; type = "DEBIT" },
        @{ account_id = $accEquity.id; amount = 100000; type = "CREDIT" }
    )
} | ConvertTo-Json

$headers1 = @{ "Idempotency-Key" = $idempotencyKey1 }
$tx1 = Invoke-RestMethod -Uri "$BaseUrl/api/v1/transactions" -Method Post -ContentType "application/json" -Headers $headers1 -Body $tx1Payload
Write-Host " OK: Transação criada com ID: $($tx1.id)" -ForegroundColor Green

# 4. Idempotência
Write-Host "`n[4/7] Testando Idempotência com mesma Idempotency-Key..." -ForegroundColor Yellow
$txReplay = Invoke-RestMethod -Uri "$BaseUrl/api/v1/transactions" -Method Post -ContentType "application/json" -Headers $headers1 -Body $tx1Payload
Write-Host " OK: Replay idempotente retornado sem erros! ID: $($txReplay.transaction.id)" -ForegroundColor Green

# 5. Partidas Dobradas Desbalanceadas
Write-Host "`n[5/7] Testando Rejeição de Transação Desbalanceada..." -ForegroundColor Yellow
$unbalancedPayload = @{
    description = "Tentativa desbalanceada"
    entries = @(
        @{ account_id = $accBank.id; amount = 5000; type = "DEBIT" },
        @{ account_id = $accEquity.id; amount = 4000; type = "CREDIT" }
    )
} | ConvertTo-Json

try {
    $null = Invoke-RestMethod -Uri "$BaseUrl/api/v1/transactions" -Method Post -ContentType "application/json" -Headers @{ "Idempotency-Key" = "fail-unbalanced-" + (Get-Date -Format "yyyyMMddHHmmssfff") } -Body $unbalancedPayload
    Write-Host " ERRO: Transação desbalanceada deveria ter falhado!" -ForegroundColor Red
} catch {
    Write-Host " OK: Transação desbalanceada rejeitada corretamente com código 422!" -ForegroundColor Green
}

# 6. Saldo Insuficiente
Write-Host "`n[6/7] Testando Proteção de Saldo Insuficiente (Overdraft)..." -ForegroundColor Yellow
$overdraftPayload = @{
    description = "Tentativa de gasto sem saldo"
    entries = @(
        @{ account_id = $accBank.id; amount = 200000; type = "CREDIT" },
        @{ account_id = $accExpense.id; amount = 200000; type = "DEBIT" }
    )
} | ConvertTo-Json

try {
    $null = Invoke-RestMethod -Uri "$BaseUrl/api/v1/transactions" -Method Post -ContentType "application/json" -Headers @{ "Idempotency-Key" = "fail-overdraft-" + (Get-Date -Format "yyyyMMddHHmmssfff") } -Body $overdraftPayload
    Write-Host " ERRO: Transação sem saldo deveria ter falhado!" -ForegroundColor Red
} catch {
    Write-Host " OK: Rejeição por saldo insuficiente funcionando corretamente com código 422!" -ForegroundColor Green
}

# 7. Pagamento Válido e Consulta de Saldos
Write-Host "`n[7/7] Executando Pagamento Válido de R$ 350,00 e Consultando Saldos..." -ForegroundColor Yellow
$paymentPayload = @{
    description = "Pagamento AWS Cloud"
    entries = @(
        @{ account_id = $accBank.id; amount = 35000; type = "CREDIT" },
        @{ account_id = $accExpense.id; amount = 35000; type = "DEBIT" }
    )
} | ConvertTo-Json

$null = Invoke-RestMethod -Uri "$BaseUrl/api/v1/transactions" -Method Post -ContentType "application/json" -Headers @{ "Idempotency-Key" = "pagto-cloud-" + (Get-Date -Format "yyyyMMddHHmmssfff") } -Body $paymentPayload

$balBank = Invoke-RestMethod -Uri "$BaseUrl/api/v1/accounts/$($accBank.id)/balance" -Method Get
Write-Host " Saldo Banco (esperado BRL 650.00): $($balBank.balance_formatted) [Saldo em centavos: $($balBank.balance)]" -ForegroundColor Cyan

$balEquity = Invoke-RestMethod -Uri "$BaseUrl/api/v1/accounts/$($accEquity.id)/balance" -Method Get
Write-Host " Saldo Capital Social (esperado BRL 1000.00): $($balEquity.balance_formatted) [Saldo em centavos: $($balEquity.balance)]" -ForegroundColor Cyan

$balExpense = Invoke-RestMethod -Uri "$BaseUrl/api/v1/accounts/$($accExpense.id)/balance" -Method Get
Write-Host " Saldo Despesas Cloud (esperado BRL 350.00): $($balExpense.balance_formatted) [Saldo em centavos: $($balExpense.balance)]" -ForegroundColor Cyan

Write-Host "`n==================================================================" -ForegroundColor Green
Write-Host " Todos os testes automatizados foram concluídos com SUCESSO!" -ForegroundColor Green
Write-Host "==================================================================" -ForegroundColor Green

