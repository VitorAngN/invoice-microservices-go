package stock

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// servidor responde com os status da lista, um por chamada (o último se repete).
func servidor(t *testing.T, chamadas *atomic.Int32, chaves *[]string, status ...int) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := int(chamadas.Add(1))
		if chaves != nil {
			*chaves = append(*chaves, r.Header.Get("Idempotency-Key"))
		}
		w.WriteHeader(status[min(n, len(status))-1])
	}))
	t.Cleanup(srv.Close)

	c := NewClient(srv.URL)
	c.Backoff = time.Millisecond // testes rápidos
	return c
}

var itens = []Item{{ProductCode: "A", Quantity: 1}}

func TestDecrementSuccess(t *testing.T) {
	var n atomic.Int32
	var chaves []string
	c := servidor(t, &n, &chaves, http.StatusOK)

	if err := c.Decrement(context.Background(), "invoice-7", itens); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if n.Load() != 1 || chaves[0] != "invoice-7" {
		t.Fatalf("esperava 1 chamada com Idempotency-Key=invoice-7, veio %d %v", n.Load(), chaves)
	}
}

func TestBusinessErrorsAreNotRetried(t *testing.T) {
	casos := map[int]error{
		http.StatusNotFound:   ErrProductNotFound,
		http.StatusConflict:   ErrInsufficientStock,
		http.StatusBadRequest: ErrInvalidRequest,
	}
	for status, esperado := range casos {
		var n atomic.Int32
		c := servidor(t, &n, nil, status)

		err := c.Decrement(context.Background(), "k", itens)
		if !errors.Is(err, esperado) {
			t.Fatalf("status %d: esperava %v, veio %v", status, esperado, err)
		}
		if n.Load() != 1 {
			t.Fatalf("status %d não deveria ser repetido, houve %d chamadas", status, n.Load())
		}
	}
}

func TestRetriesServerErrorsAndRecovers(t *testing.T) {
	var n atomic.Int32
	c := servidor(t, &n, nil, http.StatusInternalServerError, http.StatusBadGateway, http.StatusOK)

	if err := c.Decrement(context.Background(), "k", itens); err != nil {
		t.Fatalf("deveria recuperar na 3ª tentativa: %v", err)
	}
	if n.Load() != 3 {
		t.Fatalf("esperava 3 chamadas, houve %d", n.Load())
	}
}

func TestGivesUpAfterMaxAttempts(t *testing.T) {
	var n atomic.Int32
	c := servidor(t, &n, nil, http.StatusServiceUnavailable)

	err := c.Decrement(context.Background(), "k", itens)
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("esperava ErrUnavailable, veio %v", err)
	}
	if n.Load() != int32(c.MaxAttempts) {
		t.Fatalf("esperava %d tentativas, houve %d", c.MaxAttempts, n.Load())
	}
}

func TestNetworkFailureIsUnavailable(t *testing.T) {
	c := NewClient("http://127.0.0.1:1") // porta sem servidor
	c.Backoff = time.Millisecond
	c.HTTP.Timeout = 500 * time.Millisecond

	if err := c.Decrement(context.Background(), "k", itens); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("esperava ErrUnavailable, veio %v", err)
	}
}
