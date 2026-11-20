package product

import (
	"time"

	"gorm.io/gorm"
)

// Product é um item do estoque.
type Product struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Code        string    `gorm:"uniqueIndex;not null" json:"code" binding:"required,max=50"`
	Description string    `gorm:"not null" json:"description" binding:"required,max=255"`
	Balance     int       `gorm:"not null;default:0;check:balance >= 0" json:"balance" binding:"gte=0"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// StockOperation guarda as chaves de idempotência já processadas. Se o serviço
// de faturamento repetir uma baixa (retry após timeout, por exemplo), o estoque
// não é debitado duas vezes.
type StockOperation struct {
	ID        uint   `gorm:"primaryKey"`
	Key       string `gorm:"uniqueIndex;not null;size:100"`
	CreatedAt time.Time
}

// Item é uma linha da baixa de estoque.
type Item struct {
	ProductCode string `json:"product_code" binding:"required"`
	Quantity    int    `json:"quantity" binding:"required,gt=0"`
}

// DecrementRequest é o corpo de POST /api/products/decrement.
type DecrementRequest struct {
	Items []Item `json:"items" binding:"required,min=1,dive"`
}

// Migrate cria/atualiza as tabelas do serviço.
func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(&Product{}, &StockOperation{})
}
