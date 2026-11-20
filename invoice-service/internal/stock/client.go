// Package stock é o cliente HTTP do microsserviço de estoque, com retry e backoff.
package stock

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"
)

var (
	// ErrProductNotFound: o estoque respondeu 404.
	ErrProductNotFound = errors.New("produto não encontrado no estoque")
	// ErrInsufficientStock: o estoque respondeu 409.
	ErrInsufficientStock = errors.New("saldo insuficiente no estoque")
	// ErrInvalidRequest: o estoque recusou os dados (400) — não adianta repetir.
	ErrInvalidRequest = errors.New("requisição recusada pelo serviço de estoque")
	// ErrUnavailable: todas as tentativas falharam (rede fora ou erro 5xx).
	ErrUnavailable = errors.New("o serviço de estoque está indisponível no momento; a impressão da nota falhou, tente novamente mais tarde")
)

// Item é uma linha da baixa de estoque.
type Item struct {
	ProductCode string `json:"product_code"`
	Quantity    int    `json:"quantity"`
}

// Decrementer é o contrato usado pelo faturamento (permite usar um fake nos testes).
type Decrementer interface {
	Decrement(ctx context.Context, idempotencyKey string, items []Item) error
}

// Client chama POST /api/products/decrement no serviço de estoque.
type Client struct {
	BaseURL     string
	HTTP        *http.Client
	MaxAttempts int
	Backoff     time.Duration // espera antes da 2ª tentativa; dobra a cada nova falha
}

// NewClient cria um cliente com timeout por requisição (antes não havia timeout,
// e uma chamada travada podia segurar a impressão para sempre).
func NewClient(baseURL string) *Client {
	return &Client{
		BaseURL:     baseURL,
		HTTP:        &http.Client{Timeout: 5 * time.Second},
		MaxAttempts: 3,
		Backoff:     500 * time.Millisecond,
	}
}

// Decrement envia a baixa com a chave de idempotência. Erros de regra de negócio
// (404, 409, 400) voltam na hora; falhas de rede e 5xx são repetidas com backoff
// exponencial. Repetir é seguro porque o estoque ignora chaves já processadas.
func (c *Client) Decrement(ctx context.Context, idempotencyKey string, items []Item) error {
	body, err := json.Marshal(map[string]any{"items": items})
	if err != nil {
		return err
	}

	wait := c.Backoff
	for attempt := 1; attempt <= c.MaxAttempts; attempt++ {
		err := c.try(ctx, idempotencyKey, body)
		if err == nil || !retryable(err) {
			return err
		}
		log.Printf("estoque: tentativa %d/%d falhou: %v", attempt, c.MaxAttempts, err)

		if attempt < c.MaxAttempts {
			select {
			case <-ctx.Done():
				return ErrUnavailable
			case <-time.After(wait):
			}
			wait *= 2
		}
	}
	return ErrUnavailable
}

type transientError struct{ err error }

func (e transientError) Error() string { return e.err.Error() }

func retryable(err error) bool {
	var t transientError
	return errors.As(err, &t)
}

func (c *Client) try(ctx context.Context, idempotencyKey string, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/products/decrement", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return transientError{err}
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusOK:
		return nil
	case resp.StatusCode == http.StatusNotFound:
		return ErrProductNotFound
	case resp.StatusCode == http.StatusConflict:
		return ErrInsufficientStock
	case resp.StatusCode >= 500:
		return transientError{fmt.Errorf("estoque respondeu %d", resp.StatusCode)}
	default:
		return fmt.Errorf("%w (status %d)", ErrInvalidRequest, resp.StatusCode)
	}
}
