package product

import (
	"errors"
	"fmt"
	"log"
	"math/rand/v2"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// IdempotencyHeader é o header usado pelo faturamento para identificar a baixa.
const IdempotencyHeader = "Idempotency-Key"

// Handler expõe as rotas HTTP de produtos.
type Handler struct {
	DB      *gorm.DB
	Service *Service
}

// NewHandler monta o handler com suas dependências.
func NewHandler(db *gorm.DB) *Handler {
	return &Handler{DB: db, Service: &Service{DB: db}}
}

// Register registra as rotas no roteador.
func (h *Handler) Register(r gin.IRouter) {
	r.GET("/products", h.List)
	r.POST("/products", h.Create)
	r.POST("/products/decrement", h.Decrement)
	r.POST("/products/ai/generate", h.GenerateDescription)
}

// List retorna os produtos ordenados por ID.
func (h *Handler) List(c *gin.Context) {
	var products []Product
	if err := h.DB.Order("id asc").Find(&products).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao listar produtos"})
		return
	}
	c.JSON(http.StatusOK, products)
}

// Create cadastra um produto.
func (h *Handler) Create(c *gin.Context) {
	var input Product
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dados inválidos: " + err.Error()})
		return
	}

	product := Product{Code: strings.TrimSpace(input.Code), Description: input.Description, Balance: input.Balance}
	err := h.DB.Create(&product).Error
	if isDuplicateKey(err) {
		c.JSON(http.StatusConflict, gin.H{"error": "Já existe um produto com o código " + product.Code})
		return
	}
	if err != nil {
		log.Printf("erro ao criar produto: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao criar produto"})
		return
	}
	c.JSON(http.StatusCreated, product)
}

// Decrement dá baixa no estoque (chamado pelo serviço de faturamento).
func (h *Handler) Decrement(c *gin.Context) {
	var req DecrementRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dados inválidos: " + err.Error()})
		return
	}

	err := h.Service.Decrement(c.GetHeader(IdempotencyHeader), req.Items)
	switch {
	case err == nil:
		c.JSON(http.StatusOK, gin.H{"message": "Estoque atualizado com sucesso"})
	case errors.Is(err, ErrAlreadyProcessed):
		// Repetição da mesma operação: responde sucesso sem debitar de novo.
		c.JSON(http.StatusOK, gin.H{"message": "Operação já processada anteriormente"})
	case errors.Is(err, ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, ErrInsufficientBalance):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	default:
		log.Printf("erro ao debitar estoque: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro interno ao atualizar saldo"})
	}
}

type aiRequest struct {
	ProductName string `json:"product_name" binding:"required"`
}

var benefits = []string{
	"alta durabilidade e performance otimizada para o setor industrial",
	"certificação de qualidade internacional e baixo custo de manutenção",
	"design ergonômico com tecnologia de ponta e eficiência energética",
	"integração contínua com maquinário moderno e alto rendimento operacional",
}

// GenerateDescription simula (mock) um serviço de IA que sugere a descrição do produto.
func (h *Handler) GenerateDescription(c *gin.Context) {
	var req aiRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Informe product_name para gerar a descrição."})
		return
	}
	text := fmt.Sprintf("O produto %s é desenvolvido com %s.",
		strings.ToUpper(req.ProductName), benefits[rand.IntN(len(benefits))])
	c.JSON(http.StatusOK, gin.H{"description": text})
}
