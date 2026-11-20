package invoice

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/VitorAngN/invoice-microservices-go/invoice-service/internal/stock"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func init() { gin.SetMode(gin.TestMode) }

// estoqueFake registra as chamadas e devolve o erro configurado.
type estoqueFake struct {
	chamadas atomic.Int32
	chaves   sync.Map
	err      error
	espera   time.Duration
}

func (f *estoqueFake) Decrement(_ context.Context, key string, _ []stock.Item) error {
	f.chamadas.Add(1)
	f.chaves.Store(key, true)
	time.Sleep(f.espera)
	return f.err
}

func novoAmbiente(t *testing.T, fake *estoqueFake) (*gorm.DB, *gin.Engine) {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{TranslateError: true, Logger: logger.Discard})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}

	r := gin.New()
	(&Handler{DB: db, Stock: fake, FailingStock: &estoqueFake{err: stock.ErrUnavailable}}).Register(r.Group("/api"))
	return db, r
}

func chamar(r http.Handler, method, path string, body any, headers map[string]string) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

var notaValida = map[string]any{"items": []map[string]any{{"product_code": "A", "quantity": 2}}}

func criarNota(t *testing.T, r http.Handler) Invoice {
	t.Helper()
	w := chamar(r, http.MethodPost, "/api/invoices", notaValida, nil)
	if w.Code != http.StatusCreated {
		t.Fatalf("criar nota: esperava 201, veio %d: %s", w.Code, w.Body)
	}
	var inv Invoice
	_ = json.Unmarshal(w.Body.Bytes(), &inv)
	return inv
}

func statusDa(t *testing.T, db *gorm.DB, id uint) string {
	t.Helper()
	var inv Invoice
	if err := db.First(&inv, id).Error; err != nil {
		t.Fatal(err)
	}
	return inv.Status
}

func TestCreateAssignsSequentialNumbers(t *testing.T) {
	_, r := novoAmbiente(t, &estoqueFake{})
	a, b := criarNota(t, r), criarNota(t, r)
	if a.Number != 1 || b.Number != 2 || a.Status != StatusAberta {
		t.Fatalf("numeração/status inesperados: %+v %+v", a, b)
	}
}

func TestCreateValidation(t *testing.T) {
	_, r := novoAmbiente(t, &estoqueFake{})
	casos := map[string]any{
		"sem itens":           map[string]any{"items": []any{}},
		"quantidade zero":     map[string]any{"items": []map[string]any{{"product_code": "A", "quantity": 0}}},
		"quantidade negativa": map[string]any{"items": []map[string]any{{"product_code": "A", "quantity": -3}}},
		"sem código":          map[string]any{"items": []map[string]any{{"quantity": 1}}},
	}
	for nome, corpo := range casos {
		t.Run(nome, func(t *testing.T) {
			if w := chamar(r, http.MethodPost, "/api/invoices", corpo, nil); w.Code != http.StatusBadRequest {
				t.Fatalf("esperava 400, veio %d", w.Code)
			}
		})
	}
}

func TestConcurrentCreateKeepsNumbersUnique(t *testing.T) {
	db, r := novoAmbiente(t, &estoqueFake{})
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			chamar(r, http.MethodPost, "/api/invoices", notaValida, nil)
		}()
	}
	wg.Wait()

	var total, distintos int64
	db.Model(&Invoice{}).Count(&total)
	db.Model(&Invoice{}).Distinct("number").Count(&distintos)
	if total != 10 || distintos != 10 {
		t.Fatalf("esperava 10 notas com números distintos, veio total=%d distintos=%d", total, distintos)
	}
}

func TestPrintClosesInvoice(t *testing.T) {
	fake := &estoqueFake{}
	db, r := novoAmbiente(t, fake)
	inv := criarNota(t, r)

	w := chamar(r, http.MethodPost, fmt.Sprintf("/api/invoices/%d/print", inv.ID), nil, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("esperava 200, veio %d: %s", w.Code, w.Body)
	}
	if got := statusDa(t, db, inv.ID); got != StatusFechada {
		t.Fatalf("status %q, esperado Fechada", got)
	}
	if _, ok := fake.chaves.Load(fmt.Sprintf("invoice-%d", inv.ID)); !ok {
		t.Fatal("a baixa deveria usar a chave de idempotência invoice-<id>")
	}
}

func TestPrintTwiceIsRejected(t *testing.T) {
	fake := &estoqueFake{}
	_, r := novoAmbiente(t, fake)
	inv := criarNota(t, r)
	path := fmt.Sprintf("/api/invoices/%d/print", inv.ID)

	chamar(r, http.MethodPost, path, nil, nil)
	if w := chamar(r, http.MethodPost, path, nil, nil); w.Code != http.StatusConflict {
		t.Fatalf("segunda impressão: esperava 409, veio %d", w.Code)
	}
	if fake.chamadas.Load() != 1 {
		t.Fatalf("o estoque deveria ser chamado 1 vez, foi %d", fake.chamadas.Load())
	}
}

func TestConcurrentPrintDebitsStockOnce(t *testing.T) {
	fake := &estoqueFake{espera: 50 * time.Millisecond}
	_, r := novoAmbiente(t, fake)
	inv := criarNota(t, r)
	path := fmt.Sprintf("/api/invoices/%d/print", inv.ID)

	var wg sync.WaitGroup
	var ok atomic.Int32
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if chamar(r, http.MethodPost, path, nil, nil).Code == http.StatusOK {
				ok.Add(1)
			}
		}()
	}
	wg.Wait()

	if ok.Load() != 1 || fake.chamadas.Load() != 1 {
		t.Fatalf("cliques simultâneos: esperava 1 sucesso e 1 baixa, veio %d sucessos e %d baixas", ok.Load(), fake.chamadas.Load())
	}
}

func TestPrintFailureReopensInvoice(t *testing.T) {
	casos := []struct {
		nome   string
		err    error
		status int
	}{
		{"estoque fora do ar", stock.ErrUnavailable, http.StatusServiceUnavailable},
		{"saldo insuficiente", stock.ErrInsufficientStock, http.StatusConflict},
		{"produto inexistente", stock.ErrProductNotFound, http.StatusUnprocessableEntity},
	}
	for _, tc := range casos {
		t.Run(tc.nome, func(t *testing.T) {
			db, r := novoAmbiente(t, &estoqueFake{err: tc.err})
			inv := criarNota(t, r)

			w := chamar(r, http.MethodPost, fmt.Sprintf("/api/invoices/%d/print", inv.ID), nil, nil)
			if w.Code != tc.status {
				t.Fatalf("esperava %d, veio %d: %s", tc.status, w.Code, w.Body)
			}
			if got := statusDa(t, db, inv.ID); got != StatusAberta {
				t.Fatalf("após falha a nota deveria voltar para Aberta, está %q", got)
			}
		})
	}
}

func TestSimulatedFailureHeader(t *testing.T) {
	fake := &estoqueFake{}
	_, r := novoAmbiente(t, fake)
	inv := criarNota(t, r)

	w := chamar(r, http.MethodPost, fmt.Sprintf("/api/invoices/%d/print", inv.ID), nil, map[string]string{SimulateFailureHeader: "true"})
	if w.Code != http.StatusServiceUnavailable || fake.chamadas.Load() != 0 {
		t.Fatalf("falha simulada: esperava 503 sem chamar o estoque real, veio %d e %d chamadas", w.Code, fake.chamadas.Load())
	}
}

func TestPrintNotFoundAndInvalidID(t *testing.T) {
	_, r := novoAmbiente(t, &estoqueFake{})
	if w := chamar(r, http.MethodPost, "/api/invoices/999/print", nil, nil); w.Code != http.StatusNotFound {
		t.Fatalf("esperava 404, veio %d", w.Code)
	}
	if w := chamar(r, http.MethodPost, "/api/invoices/abc/print", nil, nil); w.Code != http.StatusBadRequest {
		t.Fatalf("esperava 400, veio %d", w.Code)
	}
}
