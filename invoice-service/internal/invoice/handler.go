package invoice

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/VitorAngN/invoice-microservices-go/invoice-service/internal/stock"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// SimulateFailureHeader permite demonstrar o fluxo de falha do estoque pela interface.
const SimulateFailureHeader = "X-Simulate-Failure"

// Handler expõe as rotas de notas fiscais.
type Handler struct {
	DB    *gorm.DB
	Stock stock.Decrementer
	// FailingStock é usado quando o header X-Simulate-Failure=true é enviado
	// (aponta para um endereço sem serviço, forçando retries e a resposta 503).
	FailingStock stock.Decrementer
}

// Register registra as rotas no roteador.
func (h *Handler) Register(r gin.IRouter) {
	r.GET("/invoices", h.List)
	r.POST("/invoices", h.Create)
	r.POST("/invoices/:id/print", h.Print)
}

// List retorna as notas da mais nova para a mais antiga.
func (h *Handler) List(c *gin.Context) {
	var invoices []Invoice
	if err := h.DB.Preload("Items").Order("id desc").Find(&invoices).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao listar notas"})
		return
	}
	c.JSON(http.StatusOK, invoices)
}

// Create abre uma nota com numeração sequencial.
func (h *Handler) Create(c *gin.Context) {
	var req CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "A nota deve ter ao menos 1 item, com código e quantidade maior que zero."})
		return
	}

	items := make([]InvoiceItem, len(req.Items))
	for i, it := range req.Items {
		items[i] = InvoiceItem{ProductCode: strings.TrimSpace(it.ProductCode), Quantity: it.Quantity}
	}

	// O número é MAX(number)+1. Se duas notas forem criadas ao mesmo tempo, o
	// índice único recusa a segunda, e ela tenta de novo com o próximo número.
	var inv Invoice
	var err error
	for tentativa := 0; tentativa < 5; tentativa++ {
		inv = Invoice{Status: StatusAberta, Items: items}
		err = h.DB.Transaction(func(tx *gorm.DB) error {
			var last int
			if err := tx.Model(&Invoice{}).Select("COALESCE(MAX(number), 0)").Scan(&last).Error; err != nil {
				return err
			}
			inv.Number = last + 1
			return tx.Create(&inv).Error
		})
		if !isDuplicateKey(err) {
			break
		}
		for i := range items {
			items[i].ID, items[i].InvoiceID = 0, 0
		}
	}
	if err != nil {
		log.Printf("erro ao criar nota: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao criar nota fiscal"})
		return
	}
	c.JSON(http.StatusCreated, inv)
}

// Print imprime a nota: dá baixa no estoque e muda o status para Fechada.
//
// Fluxo à prova de concorrência:
//  1. Troca Aberta → Processando com um UPDATE condicional. Só uma requisição
//     consegue; um segundo clique simultâneo recebe 409 (antes, as duas liam
//     "Aberta" e o estoque era debitado duas vezes).
//  2. Chama o estoque com a chave "invoice-<id>" (idempotente).
//  3. Sucesso → Fechada. Falha → volta para Aberta, e a nota pode ser reimpressa.
func (h *Handler) Print(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID inválido"})
		return
	}

	claim := h.DB.Model(&Invoice{}).
		Where("id = ? AND status = ?", id, StatusAberta).
		Update("status", StatusProcessando)
	if claim.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao iniciar impressão"})
		return
	}
	if claim.RowsAffected == 0 {
		var count int64
		h.DB.Model(&Invoice{}).Where("id = ?", id).Count(&count)
		if count == 0 {
			c.JSON(http.StatusNotFound, gin.H{"error": "Nota fiscal não encontrada"})
			return
		}
		c.JSON(http.StatusConflict, gin.H{"error": "Apenas notas com status 'Aberta' podem ser impressas."})
		return
	}

	var inv Invoice
	if err := h.DB.Preload("Items").First(&inv, id).Error; err != nil {
		h.reabrir(id)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao carregar a nota"})
		return
	}

	items := make([]stock.Item, len(inv.Items))
	for i, it := range inv.Items {
		items[i] = stock.Item{ProductCode: it.ProductCode, Quantity: it.Quantity}
	}

	client := h.Stock
	if c.GetHeader(SimulateFailureHeader) == "true" && h.FailingStock != nil {
		client = h.FailingStock
	}

	if err := client.Decrement(c.Request.Context(), fmt.Sprintf("invoice-%d", inv.ID), items); err != nil {
		h.reabrir(id)
		log.Printf("falha na impressão da nota %d: %v", inv.Number, err)
		c.JSON(statusFor(err), gin.H{"error": err.Error()})
		return
	}

	if err := h.DB.Model(&Invoice{}).Where("id = ?", id).Update("status", StatusFechada).Error; err != nil {
		// O estoque já foi debitado. A nota fica em Processando para conferência;
		// como a baixa é idempotente, reprocessar não debita de novo.
		log.Printf("ATENÇÃO: estoque debitado mas nota %d não foi fechada: %v", inv.Number, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Estoque atualizado, mas houve falha ao fechar a nota."})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Nota fiscal impressa e fechada com sucesso!"})
}

func (h *Handler) reabrir(id uint64) {
	if err := h.DB.Model(&Invoice{}).Where("id = ? AND status = ?", id, StatusProcessando).
		Update("status", StatusAberta).Error; err != nil {
		log.Printf("falha ao reabrir nota %d: %v", id, err)
	}
}

func statusFor(err error) int {
	switch {
	case errors.Is(err, stock.ErrProductNotFound):
		return http.StatusUnprocessableEntity
	case errors.Is(err, stock.ErrInsufficientStock):
		return http.StatusConflict
	case errors.Is(err, stock.ErrInvalidRequest):
		return http.StatusBadGateway
	default:
		return http.StatusServiceUnavailable
	}
}

func isDuplicateKey(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint failed") || strings.Contains(msg, "duplicate key")
}
