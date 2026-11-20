// Package database abre a conexão com o banco a partir das variáveis de ambiente.
package database

import (
	"fmt"
	"log"
	"os"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Config define qual banco usar. Driver "postgres" (padrão) ou "sqlite".
type Config struct {
	Driver string
	DSN    string
}

// ConfigFromEnv lê DB_DRIVER e DATABASE_URL. Para desenvolvimento rápido sem
// Docker, use DB_DRIVER=sqlite (o DSN vira o caminho do arquivo).
func ConfigFromEnv(sqliteFile string) Config {
	driver := os.Getenv("DB_DRIVER")
	if driver == "" {
		driver = "postgres"
	}
	dsn := os.Getenv("DATABASE_URL")
	if driver == "sqlite" && dsn == "" {
		dsn = sqliteFile
	}
	return Config{Driver: driver, DSN: dsn}
}

// Open conecta no banco escolhido. Diferente da versão anterior, não troca de
// banco silenciosamente: se o PostgreSQL estiver fora, o serviço falha e avisa.
func Open(cfg Config) (*gorm.DB, error) {
	gormCfg := &gorm.Config{
		TranslateError: true, // converte erros do driver em gorm.ErrDuplicatedKey etc.
		Logger:         logger.Default.LogMode(logger.Warn),
	}

	switch cfg.Driver {
	case "postgres":
		if cfg.DSN == "" {
			return nil, fmt.Errorf("DATABASE_URL não configurada")
		}
		return gorm.Open(postgres.Open(cfg.DSN), gormCfg)
	case "sqlite":
		db, err := gorm.Open(sqlite.Open(cfg.DSN), gormCfg)
		if err != nil {
			return nil, err
		}
		// SQLite aceita um escritor por vez: uma conexão evita "database is locked".
		sqlDB, err := db.DB()
		if err != nil {
			return nil, err
		}
		sqlDB.SetMaxOpenConns(1)
		log.Printf("Usando SQLite em %s (modo de desenvolvimento)", cfg.DSN)
		return db, nil
	default:
		return nil, fmt.Errorf("DB_DRIVER inválido: %q (use postgres ou sqlite)", cfg.Driver)
	}
}
