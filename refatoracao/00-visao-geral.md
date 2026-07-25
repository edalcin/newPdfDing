# Visão geral

Este documento resume o *porquê* da refatoração antes de descer aos detalhes técnicos dos demais arquivos em [refatoracao/](README.md): os três objetivos que a orientam, os princípios de desenvolvimento que todo passo de implementação deve satisfazer, as decisões arquiteturais já fechadas e o mapeamento de stack antigo → novo.

## Objetivos

1. Reescrever o newPdfDing em Go + SvelteKit espelhando a arquitetura de `pkd`, visando simplicidade e imagem Docker mínima.
2. Adotar a mesma estratégia de busca híbrida (FTS5 + embeddings Gemini + RRF) de `pkd`.
   > **Nota de escopo:** o objetivo original também citava "adotar a mesma estratégia de Storage em Amazon S3 do `pkd`". Esse item foi **retirado do escopo** por decisão do usuário durante o planejamento — ver "Assumptions & contingencies" do plano de origem e [Storage](03-storage.md). Os arquivos PDF permanecem exclusivamente no filesystem local; S3 continua existindo apenas como destino do job de backup, que já existia antes desta refatoração.
3. Preservar 100% das funcionalidades hoje em produção, **exceto** multi-usuário, tema configurável por variável de ambiente e verificação de e-mail, que são explicitamente eliminados.

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
| 2 | Processamento de PDF: pdf.js no navegador no momento do upload. O browser renderiza a página 1 para PNG (thumbnail + preview) e extrai o texto, enviando os três artefatos (PDF, PNG, texto) no mesmo `multipart/form-data`. | Mantém `CGO_ENABLED=0` — nenhuma biblioteca nativa de renderização de PDF no servidor. | Importações pela watch-dir (sem browser) usam extração de texto pura-Go e ficam sem thumbnail até a primeira abertura no viewer, quando o browser gera e faz `POST` do PNG. |
| 3 | Armazenamento: filesystem local, e só. Não existe backend S3 para os arquivos, não existe comutação de backend, não existe tela de migração de storage. | O acervo vive sob `FILES`; o único uso de S3/MinIO no produto é o destino do job de backup, que já existe hoje e permanece (bucket e credenciais próprios em `BACKUP_*`). | A abstração `storage.Backend` existe mesmo assim, com uma única implementação, porque isola a validação de caminho e o `Range` do resto do código — nenhuma interface especulativa além dela. Ver [Storage](03-storage.md). |
| 4 | Busca: caixa única com fusão RRF de FTS5 (léxico) + cosseno sobre embeddings (semântico). | Diverge de `pkd`, que mantém dois modos separados; a fusão custa ~30 linhas e melhora a UX. | Uma única caixa de busca na interface; sem seletor de modo léxico/semântico. Ver [Busca híbrida](04-busca-hibrida.md). |
| 5 | Embeddings via API Gemini (`batchEmbedContents`), igual a `pkd` — nenhum modelo local, imagem permanece mínima. Execução exclusivamente sob demanda. | Não existe worker de varredura, não existe embedding no upload, não existe fila automática. | Cada documento tem um botão "Embedar" que chama `POST /api/pdfs/{id}/embed` e embeda apenas aquele documento, naquele instante. Sem `GEMINI_API_KEY` a busca semântica fica desligada, o botão aparece desabilitado com tooltip explicando, e a busca cai para FTS5 puro. |
| 6 | Vetores como BLOB em SQLite, com cosseno calculado em Go, igual a `pkd`. | Sem `sqlite-vec`/`vss` (exigiriam CGO). | KNN é varredura completa em memória; teto documentado em [Busca híbrida](04-busca-hibrida.md). |
| 7 | UUIDv7 (`github.com/google/uuid`, `uuid.NewV7()`) para todos os IDs de entidade. | IDs ordenáveis por tempo de criação sem coluna auxiliar. | Todo `id` de tabela é `TEXT` gerado por `uuid.NewV7()`. |
| 8 | Agendamento por intervalo simples (`CONSUME_INTERVAL_MINUTES`, `BACKUP_INTERVAL_HOURS`), não cron. | Elimina dependência de parser de cron. | Esses são os únicos dois processos periódicos do produto; embedding não é um deles. |
| 9 | Workspace e WorkspaceUser são eliminados. Com usuário único, `Collection` sobe a entidade de topo. | Não há multi-usuário no produto alvo. | Coleções, incluindo a coleção padrão, permanecem como funcionalidade. Ver seção "Não-multiusuário" abaixo. |
| 10 | `PdfComment` e `PdfHighlight` fundem-se numa tabela `pdf_annotations` com coluna `kind`. | Os dois modelos Django eram subclasses da mesma abstrata `PdfAnnotation`, com campos idênticos. | Uma única tabela, um único conjunto de rotas de anotação, diferenciados por `kind IN ('comment','highlight')`. Ver [Modelo de dados](02-modelo-de-dados.md). |
| 11 | Sem migração de dados do banco Django. | A refatoração é um recomeço de schema. | Um documento descreve o script de importação única (ver `ETAPA-12-IMPORTACAO` em [ETAPAS.md](ETAPAS.md)). |

## Tabela stack antigo → novo

| Componente | Stack atual | Stack alvo |
|---|---|---|
| Roteamento HTTP | Django | chi |
| Acesso a dados | Django ORM | SQL direto com `database/sql` |
| Migrações de schema | Migrations Django | `schema.sql` embutido + migrações idempotentes de coluna |
| Processos em segundo plano | huey + supervisord | Duas goroutines com `time.Ticker` (consumo e backup, só isso) |
| Servidor de aplicação | gunicorn | `net/http` |
| Camada de apresentação | HTMX + templates Django | SvelteKit SPA |
| Arquivos estáticos | WhiteNoise | `go:embed` + `http.FileServer` |
| Build de CSS | Tailwind CLI standalone | Tailwind via Vite |
| Autenticação/sessão | allauth / sessões Django | Sessão própria em SQLite |
| Processamento de PDF | Pillow + pypdfium2 | pdf.js no navegador |
| Busca | RapidFuzz | FTS5 + embeddings sob demanda com fusão RRF |
| Cliente de object storage | minio-py | AWS SDK Go v2 (somente no backup) |
| Gerenciamento de dependências | Poetry | Go modules + npm |

## Não-multiusuário

O produto alvo é single-user. Deixam de existir por completo: `Workspace`, `WorkspaceUser`, `Profile.user`, `django.contrib.auth`, `ADMIN_EMAIL` e todo o fluxo de verificação/envio de e-mail. Resta uma única senha de acesso, `ADMIN_PASSWORD`, comparada por tempo constante — ver [Segurança](08-seguranca.md). Coleções continuam existindo como funcionalidade, mas como entidade de topo própria, sem vínculo com workspace ou usuário — ver [Modelo de dados](02-modelo-de-dados.md).
