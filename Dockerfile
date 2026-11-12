# Multi-stage build para Go
# Estágio 1: Build da aplicação compilada estaticamente
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Instala dependências de compilação
RUN apk add --no-cache git ca-certificates

# Cache inteligente de dependências do Go
COPY go.mod go.sum* ./
RUN go mod download

# Copia todo o código fonte
COPY . .

# Compila o binário estático sem dependências de CGO
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s" \
    -o /app/bin/ledger-engine \
    ./cmd/api

# Estágio 2: Imagem final mínima e segura (~15MB vs ~300MB no Node ou ~500MB no Java)
FROM alpine:3.20

WORKDIR /app

RUN apk --no-cache add ca-certificates tzdata

COPY --from=builder /app/bin/ledger-engine /app/ledger-engine

EXPOSE 8080

ENTRYPOINT ["/app/ledger-engine"]

