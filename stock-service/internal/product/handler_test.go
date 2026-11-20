package product

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func init() { gin.SetMode(gin.TestMode) }

// novoAmbiente cria um banco SQLite em memória isolado por teste.
func novoAmbiente(t *testing.T) (*gorm.DB, *gin.Engine) {
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
	NewHandler(db).Register(r.Group("/api"))
	return db, r
}

func requisicao(t *testing.T, r http.Handler, method, path string, body any, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
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

func saldo(t *testing.T, db *gorm.DB, code string) int {
	t.Helper()
	var p Product
	if err := db.Where("code = ?", code).First(&p).Error; err != nil {
		t.Fatal(err)
	}
	return p.Balance
}

func criarProduto(t *testing.T, db *gorm.DB, code string, balance int) {
	t.Helper()
	if err := db.Create(&Product{Code: code, Description: "Produto " + code, Balance: balance}).Error; err != nil {
		t.Fatal(err)
	}
}

func baixa(itens ...Item) DecrementRequest { return DecrementRequest{Items: itens} }

func TestCreateProduct(t *testing.T) {
	_, r := novoAmbiente(t)

	w := requisicao(t, r, http.MethodPost, "/api/products", map[string]any{"code": "P1", "description": "Parafuso", "balance": 10}, nil)
	if w.Code != http.StatusCreated {
		t.Fatalf("esperava 201, veio %d: %s", w.Code, w.Body)
	}

	w = requisicao(t, r, http.MethodPost, "/api/products", map[string]any{"code": "P1", "description": "Outro", "balance": 1}, nil)
	if w.Code != http.StatusConflict {
		t.Fatalf("código duplicado: esperava 409, veio %d: %s", w.Code, w.Body)
	}
}

func TestCreateProductValidation(t *testing.T) {
	_, r := novoAmbiente(t)
	casos := map[string]map[string]any{
		"sem código":     {"description": "X", "balance": 1},
		"saldo negativo": {"code": "P2", "description": "X", "balance": -5},
	}
	for nome, corpo := range casos {
		t.Run(nome, func(t *testing.T) {
			if w := requisicao(t, r, http.MethodPost, "/api/products", corpo, nil); w.Code != http.StatusBadRequest {
				t.Fatalf("esperava 400, veio %d", w.Code)
			}
		})
	}
}

func TestDecrementSuccess(t *testing.T) {
	db, r := novoAmbiente(t)
	criarProduto(t, db, "A", 10)
	criarProduto(t, db, "B", 5)

	w := requisicao(t, r, http.MethodPost, "/api/products/decrement", baixa(Item{"A", 3}, Item{"B", 5}), nil)
	if w.Code != http.StatusOK {
		t.Fatalf("esperava 200, veio %d: %s", w.Code, w.Body)
	}
	if saldo(t, db, "A") != 7 || saldo(t, db, "B") != 0 {
		t.Fatalf("saldos errados: A=%d B=%d", saldo(t, db, "A"), saldo(t, db, "B"))
	}
}

func TestDecrementIsAllOrNothing(t *testing.T) {
	db, r := novoAmbiente(t)
	criarProduto(t, db, "A", 10)
	criarProduto(t, db, "B", 1)

	// O item A tem saldo, o B não: a transação deve desfazer a baixa de A.
	w := requisicao(t, r, http.MethodPost, "/api/products/decrement", baixa(Item{"A", 3}, Item{"B", 2}), nil)
	if w.Code != http.StatusConflict {
		t.Fatalf("esperava 409, veio %d: %s", w.Code, w.Body)
	}
	if got := saldo(t, db, "A"); got != 10 {
		t.Fatalf("rollback falhou: saldo de A deveria continuar 10, está %d", got)
	}
}

func TestDecrementUnknownProduct(t *testing.T) {
	_, r := novoAmbiente(t)
	w := requisicao(t, r, http.MethodPost, "/api/products/decrement", baixa(Item{"NAO-EXISTE", 1}), nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("esperava 404, veio %d", w.Code)
	}
}

func TestDecrementRejectsInvalidQuantity(t *testing.T) {
	db, r := novoAmbiente(t)
	criarProduto(t, db, "A", 10)

	// Antes, quantidade negativa AUMENTAVA o estoque (balance - (-5)).
	for _, q := range []int{0, -5} {
		w := requisicao(t, r, http.MethodPost, "/api/products/decrement", baixa(Item{"A", q}), nil)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("quantidade %d: esperava 400, veio %d", q, w.Code)
		}
	}
	if got := saldo(t, db, "A"); got != 10 {
		t.Fatalf("saldo não deveria mudar, está %d", got)
	}
}

func TestDecrementIdempotency(t *testing.T) {
	db, r := novoAmbiente(t)
	criarProduto(t, db, "A", 10)
	headers := map[string]string{IdempotencyHeader: "invoice-1"}

	for i := 0; i < 3; i++ {
		w := requisicao(t, r, http.MethodPost, "/api/products/decrement", baixa(Item{"A", 4}), headers)
		if w.Code != http.StatusOK {
			t.Fatalf("tentativa %d: esperava 200, veio %d: %s", i+1, w.Code, w.Body)
		}
	}
	if got := saldo(t, db, "A"); got != 6 {
		t.Fatalf("mesma chave repetida 3x deveria debitar só uma vez: saldo %d, esperado 6", got)
	}
}

func TestFailedDecrementDoesNotConsumeIdempotencyKey(t *testing.T) {
	db, r := novoAmbiente(t)
	criarProduto(t, db, "A", 1)
	headers := map[string]string{IdempotencyHeader: "invoice-2"}

	if w := requisicao(t, r, http.MethodPost, "/api/products/decrement", baixa(Item{"A", 5}), headers); w.Code != http.StatusConflict {
		t.Fatalf("esperava 409, veio %d", w.Code)
	}
	db.Model(&Product{}).Where("code = ?", "A").Update("balance", 10)

	// Depois de repor o estoque, a mesma nota pode tentar de novo.
	if w := requisicao(t, r, http.MethodPost, "/api/products/decrement", baixa(Item{"A", 5}), headers); w.Code != http.StatusOK {
		t.Fatalf("esperava 200, veio %d: %s", w.Code, w.Body)
	}
	if got := saldo(t, db, "A"); got != 5 {
		t.Fatalf("saldo %d, esperado 5", got)
	}
}

func TestConcurrentDecrementNeverGoesNegative(t *testing.T) {
	db, r := novoAmbiente(t)
	criarProduto(t, db, "A", 10)

	corpo, _ := json.Marshal(baixa(Item{"A", 1}))
	var wg sync.WaitGroup
	var mu sync.Mutex
	sucessos := 0
	for i := 0; i < 25; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodPost, "/api/products/decrement", bytes.NewReader(corpo))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code == http.StatusOK {
				mu.Lock()
				sucessos++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if sucessos != 10 {
		t.Fatalf("esperava exatamente 10 baixas com sucesso, vieram %d", sucessos)
	}
	if got := saldo(t, db, "A"); got != 0 {
		t.Fatalf("saldo final %d, esperado 0", got)
	}
}

func TestListProducts(t *testing.T) {
	db, r := novoAmbiente(t)
	criarProduto(t, db, "A", 1)
	criarProduto(t, db, "B", 2)

	w := requisicao(t, r, http.MethodGet, "/api/products", nil, nil)
	var produtos []Product
	if err := json.Unmarshal(w.Body.Bytes(), &produtos); err != nil {
		t.Fatal(err)
	}
	if w.Code != http.StatusOK || len(produtos) != 2 || produtos[0].Code != "A" {
		t.Fatalf("listagem inesperada: %d %+v", w.Code, produtos)
	}
}
