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

	"github.com/VitorAngN/invoice-microservices-go/invoice-service/internal/database"
	"github.com/VitorAngN/invoice-microservices-go/invoice-service/internal/invoice"
	"github.com/VitorAngN/invoice-microservices-go/invoice-service/internal/stock"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// NewRouter monta o roteador HTTP.
func NewRouter(h *invoice.Handler) *gin.Engine {
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())
	r.Use(cors.New(cors.Config{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{"GET", "POST", "OPTIONS"},
		AllowHeaders: []string{"Origin", "Content-Type", "Accept", invoice.SimulateFailureHeader},
	}))

	r.GET("/health", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })
	h.Register(r.Group("/api"))
	return r
}

func main() {
	db, err := database.Open(database.ConfigFromEnv("faturamento.db"))
	if err != nil {
		log.Fatalf("falha ao conectar no banco de faturamento: %v", err)
	}
	if err := invoice.Migrate(db); err != nil {
		log.Fatalf("falha nas migrações: %v", err)
	}

	stockURL := os.Getenv("STOCK_SERVICE_URL")
	if stockURL == "" {
		stockURL = "http://localhost:8081"
	}
	// Cliente "quebrado" usado só na demonstração de falha (X-Simulate-Failure).
	failing := stock.NewClient("http://127.0.0.1:1")

	handler := &invoice.Handler{DB: db, Stock: stock.NewClient(stockURL), FailingStock: failing}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8082"
	}
	srv := &http.Server{Addr: ":" + port, Handler: NewRouter(handler), ReadHeaderTimeout: 5 * time.Second}

	go func() {
		log.Printf("Serviço de Faturamento rodando na porta %s (estoque em %s)", port, stockURL)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("erro no servidor: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
	log.Println("Serviço de Faturamento encerrado.")
}
