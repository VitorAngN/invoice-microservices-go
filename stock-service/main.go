package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/VitorAngN/invoice-microservices-go/stock-service/internal/database"
	"github.com/VitorAngN/invoice-microservices-go/stock-service/internal/product"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// NewRouter monta o roteador HTTP (usado também pelos testes).
func NewRouter(db *gorm.DB) *gin.Engine {
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())
	r.Use(cors.New(cors.Config{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{"GET", "POST", "OPTIONS"},
		AllowHeaders: []string{"Origin", "Content-Type", "Accept", product.IdempotencyHeader},
	}))

	r.GET("/health", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })
	product.NewHandler(db).Register(r.Group("/api"))
	return r
}

func main() {
	db, err := database.Open(database.ConfigFromEnv("estoque.db"))
	if err != nil {
		log.Fatalf("falha ao conectar no banco de estoque: %v", err)
	}
	if err := product.Migrate(db); err != nil {
		log.Fatalf("falha nas migrações: %v", err)
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}
	srv := &http.Server{Addr: ":" + port, Handler: NewRouter(db), ReadHeaderTimeout: 5 * time.Second}

	go func() {
		log.Printf("Serviço de Estoque rodando na porta %s", port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("erro no servidor: %v", err)
		}
	}()

	// Encerramento gracioso: termina as requisições em andamento antes de sair.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
	log.Println("Serviço de Estoque encerrado.")
}
