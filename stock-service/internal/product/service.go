package product

import (
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

var (
	// ErrNotFound indica que um código de produto não existe.
	ErrNotFound = errors.New("produto não encontrado")
	// ErrInsufficientBalance indica saldo menor que a quantidade pedida.
	ErrInsufficientBalance = errors.New("saldo insuficiente")
	// ErrAlreadyProcessed indica que a chave de idempotência já foi usada.
	ErrAlreadyProcessed = errors.New("operação já processada")
)

// ItemError carrega o código do produto que causou a falha.
type ItemError struct {
	Code string
	Err  error
}

func (e *ItemError) Error() string { return fmt.Sprintf("%v: %s", e.Err, e.Code) }
func (e *ItemError) Unwrap() error { return e.Err }

// Service concentra as regras de estoque, separadas do HTTP para facilitar testes.
type Service struct {
	DB *gorm.DB
}

// Decrement debita todos os itens numa única transação: ou todos são baixados,
// ou nenhum é (se o 2º item não tiver saldo, o 1º volta ao valor original).
//
// Cada baixa usa UPDATE ... SET balance = balance - q WHERE balance >= q,
// então duas requisições simultâneas nunca deixam o saldo negativo.
func (s *Service) Decrement(idempotencyKey string, items []Item) error {
	return s.DB.Transaction(func(tx *gorm.DB) error {
		if idempotencyKey != "" {
			err := tx.Create(&StockOperation{Key: idempotencyKey}).Error
			if isDuplicateKey(err) {
				return ErrAlreadyProcessed
			}
			if err != nil {
				return err
			}
		}

		for _, item := range items {
			result := tx.Model(&Product{}).
				Where("code = ? AND balance >= ?", item.ProductCode, item.Quantity).
				Update("balance", gorm.Expr("balance - ?", item.Quantity))
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 1 {
				continue
			}

			// Nenhuma linha alterada: o produto não existe ou não tem saldo.
			var count int64
			if err := tx.Model(&Product{}).Where("code = ?", item.ProductCode).Count(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				return &ItemError{Code: item.ProductCode, Err: ErrNotFound}
			}
			return &ItemError{Code: item.ProductCode, Err: ErrInsufficientBalance}
		}
		return nil
	})
}

// isDuplicateKey reconhece violação de UNIQUE no PostgreSQL e no SQLite.
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
