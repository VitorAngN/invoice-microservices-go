# Sistema de Faturamento em Microsserviços

<p>
  <img src="https://img.shields.io/badge/Status-Concluído-success?style=flat-square" alt="Status" />
  <img src="https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat-square&logo=go&logoColor=white" alt="Go" />
  <img src="https://img.shields.io/badge/Angular-17-DD0031?style=flat-square&logo=angular&logoColor=white" alt="Angular" />
  <img src="https://img.shields.io/badge/PostgreSQL-15-4169E1?style=flat-square&logo=postgresql&logoColor=white" alt="PostgreSQL" />
  <img src="https://img.shields.io/badge/Docker_Compose-Orquestração-2496ED?style=flat-square&logo=docker&logoColor=white" alt="Docker" />
</p>

Arquitetura de **microsserviços em Go** para gestão integrada de estoque e emissão de notas fiscais. O projeto foi construído com foco em resiliência e consistência de dados em operações concorrentes.

> Demonstração em vídeo: [youtube.com/watch?v=vij8uouVOGg](https://youtu.be/vij8uouVOGg)

---

## Diferenciais Técnicos

### Resiliência e Fallback
Ao emitir uma nota, o serviço de faturamento realiza uma chamada HTTP ao serviço de estoque. Caso o serviço esteja indisponível, o sistema executa **retry automático** com espera entre tentativas. Se todas as retentativas falharem, a API retorna `HTTP 503 Service Unavailable` e o frontend Angular exibe um toast de erro ao usuário.

### Consistência em Operações Concorrentes
A baixa de estoque usa verificação atômica diretamente no banco:
```sql
UPDATE products SET balance = balance - 1 WHERE id = $1 AND balance >= 1
```
Isso previne saldo negativo em compras simultâneas do mesmo item, sem depender de locks em nível de aplicação.

### Transações ACID e Rollback
Toda operação de faturamento é envolvida em uma transação de banco de dados. Em qualquer ponto de falha (rede, validação, estoque insuficiente), o `defer tx.Rollback()` garante que nenhum dado parcial seja persistido.

### Gerenciamento de Estado Reativo (Angular)
O frontend usa `BehaviorSubject` e `Observable` do **RxJS** como fonte única de verdade para os dados da UI. O operador `takeUntil(destroy$)` é usado nos componentes para cancelar as inscrições ao destruir a view, prevenindo memory leaks.

---

## Stack

| Camada | Tecnologia |
|---|---|
| Frontend | Angular 17, RxJS, CSS Glassmorphism |
| Microsserviços | Go 1.22+, Gin Framework, GORM |
| Banco de Dados | PostgreSQL 15 |
| Orquestração | Docker & Docker Compose |

---

## Como Executar

Você precisa do **Docker** e do **Go** instalados.

```bash
# 1. Subir o banco de dados PostgreSQL
docker-compose up -d

# 2. Rodar o serviço de estoque (porta 8081)
cd stock-service && go run main.go

# 3. Rodar o serviço de faturamento (porta 8082)
cd invoice-service && go run main.go

# 4. Rodar o frontend Angular (porta 4200)
cd frontend && npm install && ng serve
```

Acesse em `http://localhost:4200`.

---

## Arquitetura

```
invoice-microservices-go/
├── stock-service/       # Microsserviço de gestão de estoque (Go + GORM)
│   ├── main.go
│   ├── go.mod
│   └── go.sum
├── invoice-service/     # Microsserviço de emissão de NF com retry/fallback (Go)
│   ├── main.go
│   ├── go.mod
│   └── go.sum
├── frontend/            # Interface Angular com estado reativo (RxJS)
│   └── src/
└── docker-compose.yml   # PostgreSQL isolado para execução reproduzível
```

<img src="https://komarev.com/ghpvc/?username=VitorAngN-invoice-microservices-go" width="1" height="1" alt="" />
