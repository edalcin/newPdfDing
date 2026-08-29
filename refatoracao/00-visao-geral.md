# Visão geral

Este documento resume o *porquê* da refatoração antes de descer aos detalhes técnicos dos demais arquivos em [refatoracao/](README.md): os três objetivos que a orientam, os princípios de desenvolvimento que todo passo de implementação deve satisfazer, as decisões arquiteturais já fechadas e o mapeamento de stack antigo → novo.

## Objetivos

1. Reescrever o newPdfDing em Go + SvelteKit espelhando a arquitetura de `pkd`, visando simplicidade e imagem Docker mínima.
2. Adotar a mesma estratégia de busca híbrida (FTS5 + embeddings Gemini + RRF) de `pkd`.
   > **Nota de escopo:** o objetivo original também citava "adotar a mesma estratégia de Storage em Amazon S3 do `pkd`". Esse item foi **retirado do escopo por completo** por decisão do usuário: os arquivos PDF residem exclusivamente no filesystem local (`FILES`), sem nenhuma opção de armazenamento em nuvem. O produto **não tem nenhuma integração com Amazon S3, MinIO ou qualquer outro object storage** — inclusive o job de backup automático que hoje envia banco+arquivos a um bucket S3/MinIO foi removido nesta refatoração (ver [Funcionalidades intencionalmente removidas](10-inventario-funcionalidades.md)).
3. Preservar 100% das funcionalidades hoje em produção, **exceto** multi-usuário, tema configurável por variável de ambiente, verificação de e-mail e o backup automático para armazenamento em nuvem (S3/MinIO), que são explicitamente eliminados.

## Princípios de desenvolvimento

Cada princípio abaixo é um requisito verificável, com um critério de aceitação de uma linha.

### Versionamento

O projeto usa exclusivamente o branch `main`; nunca se criam branches novos; todo commit vai direto para `main`.

**Critério de aceitação:** `git branch --list` no repositório final mostra apenas `main`.

### Frontend

SvelteKit (SPA estática via `adapter-static`), sem framework CSS além de Tailwind, sem dependência de runtime Node em produção.

**Critério de aceitação:** `frontend/build` é servido inteiramente como arquivos estáticos por `go:embed` + `http.FileServer`.

### Dados

SQLite puro Go (`modernc.org/sqlite`), sem CGO, schema único sem múltiplos bancos.

**Critério de aceitação:** `CGO_ENABLED=0 go build` produz um binário funcional.

### Docker

Imagem final distroless, menor que 60 MB, sem shell.

**Critério de aceitação:** `docker images` mostra o tamanho e `docker run --entrypoint sh <imagem>` falha (não há shell).

### Segurança

Nenhuma vulnerabilidade CRITICAL/HIGH conhecida na imagem publicada; headers de segurança fixos; autenticação por senha única com rate limit.

**Critério de aceitação:** scan Trivy da imagem publicada retorna 0 CRITICAL/HIGH.

## Decisões arquiteturais

As onze decisões abaixo já foram tomadas e não devem ser reabertas durante a implementação.

| # | Decisão | Motivo | Consequência |
|---|---------|--------|---------------|
| 1 | Backend: Go + chi + `modernc.org/sqlite` (SQLite puro Go, sem CGO). | Binário estático em distroless, imagem ~30–50 MB contra ~400 MB hoje; espelha `pkd`. | `CGO_ENABLED=0` em todo o build; nenhuma dependência C nas camadas de dados. |
| 2 | Processamento de PDF: pdf.js no navegador no momento do upload. O browser renderiza a página 1 para PNG de preview e extrai o texto, enviando os artefatos (PDF, PNG de preview, texto) no mesmo `multipart/form-data`. Não existe thumbnail — planejado no desenho original, nunca implementado (ver [Inventário de Funcionalidades](10-inventario-funcionalidades.md)). | Mantém `CGO_ENABLED=0` — nenhuma biblioteca nativa de renderização de PDF no servidor. | Importações pela watch-dir (sem browser) usam extração de texto pura-Go e ficam sem preview até a primeira abertura no viewer, quando o browser gera e faz `POST` do PNG de preview (mesmo backfill que cobre o texto ausente — ver [API](05-api.md)). |
| 3 | Armazenamento: filesystem local, e só. Não existe backend S3 para os arquivos, não existe comutação de backend, não existe tela de migração de storage, e não há nenhuma integração do produto com Amazon S3, MinIO ou qualquer outro object storage. | O acervo vive sob `FILES`; não há backup automático para nuvem nesta refatoração — funcionalidade removida, ver [Inventário de Funcionalidades](10-inventario-funcionalidades.md). | A abstração `storage.Backend` existe mesmo assim, com uma única implementação, porque isola a validação de caminho e o `Range` do resto do código — nenhuma interface especulativa além dela. Ver [Storage](03-storage.md). |
| 4 | Busca: caixa única com fusão RRF de FTS5 (léxico) + cosseno sobre embeddings (semântico). | Diverge de `pkd`, que mantém dois modos separados; a fusão custa ~30 linhas e melhora a UX. | Uma única caixa de busca na interface; sem seletor de modo léxico/semântico. Ver [Busca híbrida](04-busca-hibrida.md). |
| 5 | Embeddings via API Gemini (`batchEmbedContents`), igual a `pkd` — nenhum modelo local, imagem permanece mínima. Execução exclusivamente sob demanda. | Não existe worker de varredura automática do acervo, não existe embedding disparado por upload, não existe agendamento por `time.Ticker` para embedding. | Cada documento tem um botão "Embedar" que chama `POST /api/pdfs/{id}/embed`; a chamada apenas **enfileira** (`202`) — um worker único em background (`internal/server/embedqueue.go`, acrescentado depois desta decisão para nunca haver duas chamadas Gemini simultâneas) drena a fila em série e embeda apenas aquele documento. O estado do job (`queued`/`extracting`/`embedding`/`done`/`failed`) é consultado via `GET /api/embed/jobs` — ver [API](05-api.md), "Embedding sob demanda". A decisão original continua de pé: nenhum embedding em massa, nenhum automatismo — um documento por acionamento explícito do usuário; a fila só serializa acionamentos, não os cria. Sem `GEMINI_API_KEY` a busca semântica fica desligada, o botão aparece desabilitado com tooltip explicando, e a busca cai para FTS5 puro. |
| 6 | Vetores como BLOB em SQLite, com cosseno calculado em Go, igual a `pkd`. | Sem `sqlite-vec`/`vss` (exigiriam CGO). | KNN é varredura completa em memória; teto documentado em [Busca híbrida](04-busca-hibrida.md). |
| 7 | UUIDv7 (`github.com/google/uuid`, `uuid.NewV7()`) para todos os IDs de entidade. | IDs ordenáveis por tempo de criação sem coluna auxiliar. | Todo `id` de tabela é `TEXT` gerado por `uuid.NewV7()`. |
| 8 | Agendamento por intervalo simples (`CONSUME_INTERVAL_MINUTES`), não cron. | Elimina dependência de parser de cron. | Este é o único processo periódico do produto; embedding não é um deles, e não há job de backup nesta refatoração. |
| 9 | Workspace e WorkspaceUser são eliminados. Com usuário único, o plano original previa que `Collection` subisse à entidade de topo. | Não há multi-usuário no produto alvo. | `Workspace`/`WorkspaceUser` foram de fato eliminados. **Coleções nunca foram implementadas**: a tabela `collections`, as rotas `/api/collections` e a tela `/collections` ficaram apenas no plano — o schema e a API de produção não têm nenhum rastro de coleção. Ver [Inventário de Funcionalidades](10-inventario-funcionalidades.md). |
| 10 | `PdfComment` e `PdfHighlight` fundem-se numa tabela `pdf_annotations` com coluna `kind`. | Os dois modelos Django eram subclasses da mesma abstrata `PdfAnnotation`, com campos idênticos. | Uma única tabela, um único conjunto de rotas de anotação, diferenciados por `kind IN ('comment','highlight')`. Ver [Modelo de dados](02-modelo-de-dados.md). |
| 11 | Sem migração automática de dados do banco Django — a refatoração é um recomeço de schema, feito à parte de qualquer sincronização contínua. | A refatoração é um recomeço de schema. | Foi implementado um comando único de importação (`internal/server/import_legacy.go`, `-import-legacy`), executado uma vez contra o banco de produção. Depois da migração, nesta sessão, o comando e o código de importação foram **removidos do binário** — não existe mais caminho de importação disponível. O mapeamento de campos permanece documentado, como histórico, em [Modelo de dados](02-modelo-de-dados.md). |

## Tabela stack antigo → novo

| Componente | Stack atual | Stack alvo |
|---|---|---|
| Roteamento HTTP | Django | chi |
| Acesso a dados | Django ORM | SQL direto com `database/sql` |
| Migrações de schema | Migrations Django | `schema.sql` embutido + migrações idempotentes de coluna |
| Processos em segundo plano | huey + supervisord | Uma goroutine com `time.Ticker` (consumo por watch-dir) |
| Servidor de aplicação | gunicorn | `net/http` |
| Camada de apresentação | HTMX + templates Django | SvelteKit SPA |
| Arquivos estáticos | WhiteNoise | `go:embed` + `http.FileServer` |
| Build de CSS | Tailwind CLI standalone | Tailwind via Vite |
| Autenticação/sessão | allauth / sessões Django | Sessão própria em SQLite |
| Processamento de PDF | Pillow + pypdfium2 | pdf.js no navegador |
| Busca | RapidFuzz | FTS5 + embeddings sob demanda com fusão RRF |
| Gerenciamento de dependências | Poetry | Go modules + npm |

## Não-multiusuário

O produto alvo é single-user. Deixam de existir por completo: `Workspace`, `WorkspaceUser`, `Profile.user`, `django.contrib.auth`, `ADMIN_EMAIL` e todo o fluxo de verificação/envio de e-mail. Resta uma única senha de acesso, `ADMIN_PASSWORD`, comparada por tempo constante — ver [Segurança](08-seguranca.md). O plano original também previa que Coleções subissem a uma entidade de topo própria, sem vínculo com workspace ou usuário — essa parte do plano **nunca foi implementada**: não existe tabela `collections` nem rota `/api/collections` no produto (ver [Inventário de Funcionalidades](10-inventario-funcionalidades.md)).
