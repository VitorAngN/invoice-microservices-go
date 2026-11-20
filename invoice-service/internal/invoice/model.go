// Package invoice contém as notas fiscais e o fluxo de impressão.
package invoice

import (
	"time"

	"gorm.io/gorm"
)

// Status possíveis da nota.
const (
	StatusAberta      = "Aberta"
	StatusProcessando = "Processando" // impressão em andamento (trava contra impressão dupla)
	StatusFechada     = "Fechada"
)

// Invoice é a nota fiscal.
type Invoice struct {
	ID        uint          `gorm:"primaryKey" json:"id"`
	Number    int           `gorm:"uniqueIndex" json:"number"`
	Status    string        `gorm:"not null;default:'Aberta'" json:"status"`
	Items     []InvoiceItem `gorm:"foreignKey:InvoiceID" json:"items"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
}

// InvoiceItem é uma linha da nota.
type InvoiceItem struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	InvoiceID   uint      `gorm:"not null" json:"-"`
	ProductCode string    `gorm:"not null" json:"product_code"`
	Quantity    int       `gorm:"not null" json:"quantity"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// CreateItemRequest valida cada item recebido na criação da nota.
type CreateItemRequest struct {
	ProductCode string `json:"product_code" binding:"required"`
	Quantity    int    `json:"quantity" binding:"required,gt=0"`
}

// CreateRequest é o corpo de POST /api/invoices.
type CreateRequest struct {
	Items []CreateItemRequest `json:"items" binding:"required,min=1,dive"`
}

// Migrate cria/atualiza as tabelas do serviço.
func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(&Invoice{}, &InvoiceItem{})
}
