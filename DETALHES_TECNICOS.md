<h1 align="center">Detalhamento Técnico — Sistema Korp</h1>

<p align="center">
  <img src="https://img.shields.io/badge/Status-Componentizado-success?style=for-the-badge" alt="Status" />
  <img src="https://img.shields.io/badge/Angular-DD0031?style=for-the-badge&logo=angular&logoColor=white" alt="Angular" />
  <img src="https://img.shields.io/badge/Go-00ADD8?style=for-the-badge&logo=go&logoColor=white" alt="Go" />
  <img src="https://img.shields.io/badge/PostgreSQL-4169E1?style=for-the-badge&logo=postgresql&logoColor=white" alt="PostgreSQL" />
  <img src="https://img.shields.io/badge/Docker-2496ED?style=for-the-badge&logo=docker&logoColor=white" alt="Docker" />
</p>

> [!IMPORTANT]  
> **Arquitetura de Microsserviços Resilientes:**  
> Este documento detalha as decisões arquiteturais e técnicas tomadas para o desenvolvimento do sistema de Emissão de Notas Fiscais, seguindo rigorosamente os requisitos do desafio técnico da Korp.

---

## 1. Arquitetura do Sistema

O sistema foi desenhado sob o paradigma de **Microsserviços**, permitindo escalabilidade isolada e separação de interesses (*Separation of Concerns*).

- **Frontend (SPA):** Interface reativa em Angular 17 que centraliza a experiência do usuário.
- **Stock Service (Porta 8081):** Domínio de Estoque. Responsável por CRUD de produtos, controle de saldos e integração com IA.
- **Invoice Service (Porta 8082):** Domínio de Faturamento. Responsável pela emissão de notas e orquestração de abatimento de estoque.

---

## 2. Frontend (Angular 17)

### Ciclos de Vida (LifeCycle Hooks)
Utilizamos os hooks nativos para garantir a integridade dos dados e performance:
- **`ngOnInit`**: Utilizado em nossos componentes (`ProductsComponent`, `InvoicesComponent`) para inicializar as subscrições reativas e carregar a lista inicial de dados provenientes do Backend.
- **`ngOnDestroy`**: Essencial para a saúde da aplicação. Implementamos um padrão de destruição usando um `Subject` (`destroy$`) combinado com o operador `takeUntil`. Isso garante que, ao navegar para outra tela, todos os observables sejam encerrados, prevenindo **Memory Leaks**.

### Gerenciamento Reativo com RxJS
O RxJS não é apenas uma biblioteca de HTTP, mas o "cérebro" da aplicação:
- **`BehaviorSubject`**: Usado no `ApiService` para manter o estado atual dos produtos e notas fiscais. Isso permite que qualquer componente que injete o serviço tenha acesso imediato ao estado atualizado sem precisar de novas batidas manuais na API.
- **Operadores (`pipe`, `map`, `catchError`)**: Toda resposta é tratada reativamente. Criamos fluxos onde um erro de rede dispara automaticamente uma lógica de feedback visual no Angular.

### Design & Visual
- **Glassmorphism**: Implementamos um sistema visual moderno usando **CSS Puro** e variáveis CSS, garantindo um código leve (sem frameworks pesados de UI) e visualmente premium.
- **Standalone Components**: Seguindo a recomendação mais recente do Angular Core, eliminamos a necessidade de `NgModules`, tornando o bundle final menor e o código mais legível.

---

## 3. Backend (Golang)

### Gerenciamento de Dependências (Go Modules)
Utilizamos o padrão **Go Modules** para isolar as dependências de cada microsserviço. Cada pasta (`stock-service` e `invoice-service`) possui seu próprio `go.mod`, garantindo que versões conflitantes de bibliotecas nunca afetem o sistema como um todo.

### Frameworks e Performance
- **Gin Gonic**: Escolhido pela sua altíssima performance em roteamento HTTP e facilidade de middleware.
- **GORM**: Utilizado para abstração do banco de dados PostgreSQL, permitindo migrations automáticas e consultas seguras via Structs nativas do Go.

### Resiliência e Tolerância a Falhas
Um dos pontos chave do projeto é a comunicação entre Faturamento e Estoque:
- **Padrão Retry com Backoff**: Quando o Faturamento tenta dar baixa no estoque, ele utiliza um client HTTP customizado que executa **Retentativas (Retries)** caso o serviço de estoque esteja momentaneamente instável.
- **Rollback de Transações**: Caso o abatimento do estoque falhe após todas as tentativas, a transação no banco de dados do Faturamento sofre um **Rollback**, mantendo a consistência dos dados do usuário (Atomicidade).

---

## 4. Persistência e Segurança

### Concorrência e Atomicidade
Para o requisito de concorrência no estoque, implementamos uma query atômica no banco:
```sql
UPDATE products SET balance = balance - ?, updated_at = ? WHERE id = ? AND balance >= ?
```
Isso garante que, mesmo sob carga massiva de requisições simultâneas, nunca teremos saldo negativo ou condições de corrida (*Race Conditions*).

### Bancos de Dados Isolados
Cada serviço possui seu próprio schema/container de banco de dados no PostgreSQL, respeitando o princípio de microsserviços onde um serviço não "invade" a base do outro diretamente (apenas via API).

---

## 5. Diferencial IA

Implementamos um serviço de **IA Generativa de Descrições**. No cadastro de produtos, o usuário pode preencher apenas o código e clicar no botão "IA".
O Backend então processa o contexto do produto e gera uma descrição técnica detalhada automaticamente, demonstrando como integrar modelos de linguagem em fluxos de negócios reais.

---

## Vídeo de Demonstração
Para visualizar o sistema em funcionamento e o detalhamento das funcionalidades, acesse o link abaixo:
**[Assistir Demonstração no YouTube](https://youtu.be/vij8uouVOGg)**

---

<p align="center">
  Desenvolvido com foco em excelência técnica por <strong>Vitor</strong>.
</p>
