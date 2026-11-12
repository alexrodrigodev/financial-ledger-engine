package config

import (
	"os"
)

// Config armazena as configurações da aplicação carregadas de variáveis de ambiente.
// Em Go, não usamos anotações como no Spring (@Value) nem process.env solto por todo o código como em Node.
// Concentramos a configuração em uma struct fortemente tipada.
type Config struct {
	Port        string
	DatabaseURL string
	Environment string
}

func Load() *Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://postgres:postgres@localhost:5432/ledger_db?sslmode=disable"
	}

	env := os.Getenv("APP_ENV")
	if env == "" {
		env = "development"
	}

	return &Config{
		Port:        port,
		DatabaseURL: dbURL,
		Environment: env,
	}
}

