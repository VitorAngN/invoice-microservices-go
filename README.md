<h1 align="center">Invoice Microservices — Sistema de Faturamento</h1>

<p align="center">
  <a href="https://github.com/VitorAngN/invoice-microservices-go/actions/workflows/ci.yml"><img src="https://github.com/VitorAngN/invoice-microservices-go/actions/workflows/ci.yml/badge.svg" alt="CI" /></a>
  <img src="https://img.shields.io/badge/Go-1.26-00ADD8?style=flat-square&logo=go" />
  <img src="https://img.shields.io/badge/Angular-17-DD0031?style=flat-square&logo=angular" />
  <img src="https://img.shields.io/badge/PostgreSQL-15-4169E1?style=flat-square&logo=postgresql" />
  <img src="https://img.shields.io/badge/Docker-Ready-2496ED?style=flat-square&logo=docker" />
</p>

<p align="center">
  <a href="https://vitorangn.github.io/invoice-microservices-go/"><b>Página do projeto</b></a> ·
  <a href="https://vitorangn.github.io/invoice-microservices-go/api/"><b>Documentação da API</b></a>
</p>

Sistema de faturamento e controle de estoque desenvolvido com microsserviços em Go, PostgreSQL e frontend em Angular.

O projeto simula um fluxo de emissão de nota/faturamento integrado ao controle de estoque, com foco em comunicação HTTP entre serviços, persistência relacional, tratamento de falhas, transações ACID, rollback, retry automático, fallback HTTP 503 e atualização atômica de estoque para reduzir inconsistências em operações concorrentes.

O ambiente local utiliza Docker Compose para o banco PostgreSQL, enquanto os serviços Go e o frontend Angular podem ser executados separadamente para facilitar testes e desenvolvimento.

Para detalhes aprofundados sobre as decisões técnicas, uso de RxJS, Ciclos de Vida do Angular e Resiliência do Backend, acesse o guia:  
**[DETALHES_TECNICOS.md](./DETALHES_TECNICOS.md)**

### Vídeo de Demonstração
Acesse a apresentação em vídeo do sistema e resumo das funcionalidades:  
**[Assistir no YouTube](https://youtu.be/vij8uouVOGg)**

---


## Como Rodar o Projeto

### Opção 1: tudo com Docker (banco + 2 microsserviços)
```bash
docker compose up -d --build
```
- Estoque: http://localhost:8081 · Faturamento: http://localhost:8082 · `GET /health` em cada um.

Depois, rode o frontend:
```bash
cd frontend
npm install
npx ng serve      # http://localhost:4200
```

### Opção 2: serviços Go localmente
```bash
docker compose up -d db                      # só o PostgreSQL
cd stock-service   && go run .              # porta 8081
cd invoice-service && go run .              # porta 8082
```
Para testar sem PostgreSQL, use SQLite explicitamente: `DB_DRIVER=sqlite go run .`

### Testes
```bash
cd stock-service   && go test -race ./...
cd invoice-service && go test -race ./...
```
Os testes usam SQLite em memória e `httptest`, sem precisar de banco ou rede, e rodam no GitHub Actions a cada push.

---

## Decisões Técnicas

### 1. Ciclos de vida do Angular utilizados
Foi feito uso de dois ciclos de vida primários nos componentes standalone (`ProductsComponent` e `InvoicesComponent`):
- `ngOnInit()`: Invocado em todas as invocações de tela para carregar as inscrições nos observables de Faturamento/Produtos e requerer os dados iniciais (`loadProducts()`).
- `ngOnDestroy()`: Usado criticamente em conjunto com uma técnica de desinscrição (`Subject/takeUntil`) para prevenir **Memory Leaks** por observables que ficam "vivos" após o fechamento da tela.

### 2. Uso da biblioteca RxJS
O **RxJS** foi massivamente utilizado como a espinha dorsal de gerência de estado e eventos assíncronos:
- Uso do `BehaviorSubject` e `Observable` no `ApiService` para agirem como a única "Fonte de Verdade" dos dados da UI (State Management centralizado).
- Funções operator (`pipe`, `tap` e `catchError`) para manipular as respostas HTTP (disparar retentativas ou emular lógicas antes que o subscriber da View receba a info).
- `takeUntil(this.destroy$)` para gerenciar a destruição automatizada das subinscrições.

### 3. Bibliotecas Adicionais no Frontend e Visual
- Utilizamos o core nativo de Angular como `CommonModule`, e `ReactiveFormsModule` para validações ativas em tempo real no HTML e criação de sub-formulários (uma nota com N produtos - `FormArray`).
- Para os **componentes visuais**: A abordagem escolhida não importou pesados frameworks visuais, em vez disso, codificou-se um design ultra-veloz e nativo focando na tendência de _Glassmorphism_ em variáveis CSS Puras com Google Fonts (Outfit).

### 4. Gerenciamento de Dependências no Golang
Todo o microsserviço Golang é isolado com os padrões do `Go Modules`. Foram inicializados ecossistemas independentes (`go mod init`) alocados na pasta raiz em cada projeto contendo as descrições em `go.mod` e as resoluções de hashes em `go.sum`, determinizando os sub-packages.

### 5. Utilização de Frameworks no Golang
Ambos serviços usam um esqueleto semelhante para as APIs que importam **Gin Web Framework** para roteamento extremante veloz, e o **GORM** (Go Object Relational Mapper) para traduzir *Structs* nativas do Go nas migrações lógicas do PostgreSQL usando o driver oficial da pg. O tratamento de origens foi com a biblioteca local `cors`.

### 6. Tratamento de Erros e Resiliência (Backend)
- **Retry com backoff exponencial:** o faturamento chama o estoque com timeout por requisição e até 3 tentativas (0,5 s → 1 s). Falhas de rede e respostas 5xx são repetidas; erros de regra de negócio (404 produto inexistente, 409 saldo insuficiente, 400) voltam na hora, porque repetir não mudaria o resultado.
- **Fallback HTTP 503:** se todas as tentativas falharem, a API responde `503 Service Unavailable` e o Angular mostra um aviso amigável. O header `X-Simulate-Failure: true` permite demonstrar esse cenário pela interface.
- **Erros tipados:** cada falha do estoque vira um erro Go (`ErrInsufficientStock`, `ErrProductNotFound`, `ErrUnavailable`) mapeado para o status HTTP correto.

### 7. Concorrência, Atomicidade e Idempotência
- **Baixa atômica e "tudo ou nada":** todos os itens de uma nota são debitados numa única transação com `UPDATE ... SET balance = balance - q WHERE balance >= q`. Se um item não tiver saldo, nenhum é debitado, e o saldo nunca fica negativo, mesmo com requisições simultâneas (há teste com 25 baixas concorrentes sobre saldo 10).
- **Impressão única:** a impressão troca o status `Aberta → Processando` com um UPDATE condicional. Se dois cliques chegarem ao mesmo tempo, só um consegue a troca e o outro recebe `409`. Se o estoque falhar, a nota volta para `Aberta`.
- **Idempotência:** cada baixa envia `Idempotency-Key: invoice-<id>`. O estoque grava a chave na mesma transação da baixa; se o faturamento repetir a chamada (por exemplo, a resposta se perdeu na rede), o estoque responde sucesso sem debitar de novo.
- **Numeração da nota:** `MAX(number) + 1` protegido por índice único; em caso de conflito entre duas criações simultâneas, a criação é refeita com o próximo número.
- **Validação:** quantidades precisam ser maiores que zero (antes, uma quantidade negativa aumentava o estoque).

### 8. Estrutura dos Serviços
```
stock-service/ e invoice-service/
├── main.go                 # configuração, rotas e encerramento gracioso
└── internal/
    ├── database/           # conexão (PostgreSQL ou SQLite explícito)
    ├── product|invoice/    # modelos, regras e handlers HTTP + testes
    └── stock/              # (faturamento) cliente HTTP do estoque com retry + testes
```
Dependências são injetadas nos handlers (sem variável global de banco), o que permite testar cada regra isoladamente.
