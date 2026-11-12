package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"financial-ledger-engine/internal/config"
	"financial-ledger-engine/internal/repository/postgres"
	transportHttp "financial-ledger-engine/internal/transport/http"
)

func main() {
	// Logger estruturado nativo do Go
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	slog.Info("Inicializando Financial Ledger Engine...")

	// Carrega variáveis de ambiente
	cfg := config.Load()

	// Conecta ao PostgreSQL com mecanismo de retry resiliente para containers Docker
	db, err := connectWithRetry(cfg.DatabaseURL, 10, 2*time.Second)
	if err != nil {
		slog.Error("Falha crítica ao conectar com PostgreSQL após múltiplas tentativas", "erro", err)
		os.Exit(1)
	}
	defer db.Close()

	// Inicializa a camada de repositório PostgreSQL
	repo := postgres.NewRepository(db)

	// Garante que o schema inicial esteja criado
	initCtx, initCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer initCancel()
	if err := repo.InitSchema(initCtx); err != nil {
		slog.Warn("Aviso ao inicializar schema (pode já existir via migração)", "erro", err)
	} else {
		slog.Info("Schema do banco de dados verificado com sucesso")
	}

	// Inicializa camada de transporte HTTP
	handler := transportHttp.NewHandler(repo)
	router := transportHttp.NewRouter(handler)

	server := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.Port),
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown: escuta interrupção do SO (SIGINT, SIGTERM)
	shutdownChan := make(chan os.Signal, 1)
	signal.Notify(shutdownChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		slog.Info("Servidor HTTP pronto para receber requisições",
			"porta", cfg.Port,
			"ambiente", cfg.Environment,
		)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("Falha fatal na execução do servidor HTTP", "erro", err)
			os.Exit(1)
		}
	}()

	sig := <-shutdownChan
	slog.Info("Sinal de desligamento recebido, encerrando servidor...", "sinal", sig.String())

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("Erro durante o desligamento forçado do servidor", "erro", err)
		os.Exit(1)
	}

	slog.Info("Financial Ledger Engine finalizado com sucesso!")
}

// connectWithRetry tenta conectar ao banco de dados com backoff para resiliência no boot de containers.
func connectWithRetry(databaseURL string, maxRetries int, interval time.Duration) (*sql.DB, error) {
	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		db, err := postgres.OpenDB(databaseURL)
		if err == nil {
			slog.Info("Conexão com PostgreSQL estabelecida com sucesso", "tentativa", attempt)
			return db, nil
		}
		lastErr = err
		slog.Warn("Aguardando disponibilidade do PostgreSQL...",
			"tentativa", attempt,
			"max_tentativas", maxRetries,
			"proxima_tentativa_em", interval.String(),
			"erro", err.Error(),
		)
		time.Sleep(interval)
	}
	return nil, lastErr
}
