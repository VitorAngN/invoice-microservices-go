<h1 align="center">Invoice Microservices — Sistema de Faturamento</h1>

<p align="center">
  <img src="https://img.shields.io/badge/Status-Concluído-success?style=flat-square" />
  <img src="https://img.shields.io/badge/Go-1.22-00ADD8?style=flat-square&logo=go" />
  <img src="https://img.shields.io/badge/Angular-17-DD0031?style=flat-square&logo=angular" />
  <img src="https://img.shields.io/badge/PostgreSQL-15-4169E1?style=flat-square&logo=postgresql" />
  <img src="https://img.shields.io/badge/Docker-Ready-2496ED?style=flat-square&logo=docker" />
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

Você precisa do **Docker** e do **Go** instalados na sua máquina.

1. **Subir o Banco de Dados PostgreSQL:**
   Na raiz do projeto (onde está o `docker-compose.yml`), rode:
   ```bash
   docker-compose up -d
   ```
   Os serviços usam a variável `DATABASE_URL`. Se ela não estiver configurada ou o PostgreSQL estiver indisponível, o projeto cai para SQLite local como fallback de desenvolvimento.
2. **Rodar Serviço de Estoque (Porta 8081):**
   ```bash
   cd stock-service
   go run main.go
   ```
3. **Rodar Serviço de Faturamento (Porta 8082):**
   ```bash
   cd invoice-service
   go run main.go
   ```
4. **Rodar Frontend Angular (Porta 4200):**
   ```bash
   cd frontend
   npm install
   ng serve
   ```
   *Acesse `http://localhost:4200`.*


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

### 6. Tratamento de Erros e Exceções (Backend)
Para o cenário de falha, usamos o `Begin()` limitador do ORM (Transações ACID).
- Ex: Ao iniciar a impressão de NF e disparar a chamada à API no serviço de Estoque, a rede pode cair ou ocorrer indisponibilidade. Faturamento vai tratar usando Retry local de retentativa em x segundos.
- Se todas as retentativas falharem (Feedback), a API devolve um Http `503 Service Unavailable`, o Angular apanha no RxJS e devolve um toast amigável: "O serviço de estoque está indisponível. A impressão falhou.".
- Ao dar erro (em qualquer parte), usamos o `defer tx.Rollback()`, para assegurar a atomicidade, e as exceptions de negócio geram retornos em JSON (`http.StatusConflict` -> `"error": "Saldo insuficiente"`).

### 7. Trabalhos Extras Realizados (Concorrência e Idempotência)
Para reduzir inconsistências em operações concorrentes, a atualização de estoque utiliza uma operação atômica no banco de dados:

UPDATE balance = balance - 1 WHERE balance >= 1

Essa abordagem evita que múltiplas requisições consumam o mesmo item quando o saldo disponível é insuficiente.

Também foi implementado um controle básico de status da nota para reduzir o risco de reemissão acidental durante o fluxo de faturamento.
