# Docker, CI e Deploy

Este documento fixa o conteúdo alvo de empacotamento e entrega: o `Dockerfile` de três estágios, o `.dockerignore`, o workflow de CI/CD (`.github/workflows/docker-publish.yaml`), o `.github/dependabot.yml`, a lista fechada de variáveis de ambiente — incluindo as eliminadas na migração —, o `.env.example` e o conteúdo alvo de `UNRAID.md`. É o dono de todo `nome | obrigatória | default | significado` de configuração do produto; nenhum outro documento define uma variável de ambiente nova.

Para a árvore de diretórios e as dependências Go fixadas, ver [Arquitetura](01-arquitetura.md). Para o detalhamento dos headers de segurança, rate limiting e o checklist de segurança que cita a imagem `nonroot`, ver [Segurança](08-seguranca.md). Para a lista do que é apagado/reescrito no repositório atual, ver [Limpeza do repositório](09-limpeza-repositorio.md).

## Dockerfile de três estágios

Três estágios, na ordem:

1. **`node:22-alpine`** — `npm ci` (inclui `pdfjs-dist` 5.5.207, já fixado em `frontend/package.json`), depois `npm run build` → executa `frontend/scripts/copy-pdfjs.mjs` (copia só o necessário de `node_modules/pdfjs-dist` — motor, `cmaps/`, `standard_fonts/` — para `frontend/static/pdfjs/`; `viewer.html`/`viewer.mjs`/`viewer.css` já são código do repositório, não baixados) e então `vite build` → `frontend/build`.
2. **`golang:1.25-alpine`** — copia o build do frontend para `internal/server/web/dist`, compila com `CGO_ENABLED=0`, `-trimpath`, `-ldflags="-s -w"`.
3. **`gcr.io/distroless/static-debian12:nonroot`** — copia só o binário; `USER nonroot`; `EXPOSE 8000`; `ENTRYPOINT ["/newpdfding"]`; `HEALTHCHECK` via flag `-healthcheck` do próprio binário (a imagem distroless não tem shell nem `curl`/`wget`, então o healthcheck não pode ser um comando de shell).

**Meta de tamanho declarada: < 60 MB** (o stack Django atual gera ~400 MB). O principal peso da imagem final passa a ser o pdf.js embutido, não o runtime — daí a limpeza de `web/locale` e `web/standard_fonts` no estágio 1.

Estrutura de comentário de seção (`# ── Stage N: … ──`), `WORKDIR` e `COPY --from=` seguem o estilo do `Dockerfile` de referência de `pkd` (`D:/git/pkd/Dockerfile`, somente leitura). Divergências deliberadas em relação a essa referência: nome do binário `newpdfding` (não `pkd`), porta `8000` exposta (não `8080`), tag final `:nonroot` explícita e diretiva `USER nonroot` declarada (a referência usa a tag sem `:nonroot` e não declara `USER` — aqui é obrigatório por ser requisito de segurança, ver [Segurança](08-seguranca.md)).

```dockerfile
# ── Stage 1: Frontend build (Node + pdf.js) ─────────────────────────────────
FROM node:22-alpine AS frontend

WORKDIR /app/frontend

COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci

COPY frontend/ .

# pdf.js 5.5.207 já é uma dependência declarada em package.json (pdfjs-dist)
# — nada para baixar aqui; npm run build chama frontend/scripts/copy-pdfjs.mjs
# antes do vite build (ver 06-frontend.md, "Viewer — ponte postMessage").
RUN npm run build
# Saída: /app/frontend/build/  (adapter-static)



# ── Stage 2: Go build ────────────────────────────────────────────────────────
FROM golang:1.25-alpine AS build

WORKDIR /src

RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=frontend /app/frontend/build/ ./internal/server/web/dist/

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /out/newpdfding ./cmd/newpdfding


# ── Stage 3: Runtime (distroless, não-root) ──────────────────────────────────
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/newpdfding /newpdfding

USER nonroot

EXPOSE 8000

HEALTHCHECK --interval=30s --timeout=3s --retries=3 \
    CMD ["/newpdfding", "-healthcheck"]

ENTRYPOINT ["/newpdfding"]
```

## `.dockerignore`

```gitignore
.git
node_modules
frontend/build
internal/server/web/dist
*.md
refatoracao/
```

`frontend/build` e `internal/server/web/dist` ficam de fora do contexto de build porque são saída gerada — o estágio 1 do `Dockerfile` os reconstrói do zero a cada build, e nunca são versionados. `refatoracao/` e `*.md` ficam de fora porque são planejamento/documentação, não código de produção.

## Workflow de CI/CD — `.github/workflows/docker-publish.yaml`

- **Gatilho**: `push` em `main` e `workflow_dispatch` (disparo manual). Sem gatilho `pull_request` — o repositório mantém um único branch, `main` (ver regra do projeto); não há branch de feature para validar antes do merge.
- **Job `test`**: Node (`actions/setup-node`, `npm ci` + `npm run build` do frontend, para garantir que o build do SvelteKit não quebrou) seguido de Go (`actions/setup-go`, `go vet ./...`, `go test ./...`, `govulncheck`).
- **Job `publish`**: depende de `test` (`needs: test`); usa `docker/setup-buildx-action`, `docker/login-action` em `ghcr.io`, `docker/metadata-action` para gerar as tags `latest` e `sha-<short>`, e `docker/build-push-action` para `linux/amd64`, publicando em `ghcr.io/edalcin/newpdfding`. `permissions` do workflow: `contents: read`, `packages: write`.
- **Scan Trivy**: roda depois do push da imagem, `exit-code: 1` para severidade `CRITICAL,HIGH` — **falha o pipeline** se encontrar vulnerabilidade dessa severidade. Mais rígido que a referência `pkd` (`D:/git/pkd/.github/workflows/build-and-publish.yml`, somente leitura), que roda o mesmo `aquasecurity/trivy-action` mas com `exit-code: 0` (só reporta, nunca falha o job).

Estrutura de referência (`pkd`): job `test` roda `actions/setup-node` + `npm install`/`npm run build` do frontend antes de `actions/setup-go` + `go test ./... -timeout 120s` + `go vet ./...` + `govulncheck`; job de publicação usa `docker/setup-buildx-action`, `docker/login-action` em `ghcr.io`, `docker/metadata-action` para tags, `docker/build-push-action` com `cache-from`/`cache-to type=gha`, e por fim `aquasecurity/trivy-action@master` sobre a imagem publicada.

```yaml
name: Docker Publish

on:
  push:
    branches: [main]
  workflow_dispatch:

env:
  REGISTRY: ghcr.io
  IMAGE_NAME: edalcin/newpdfding

permissions:
  contents: read
  packages: write

jobs:
  test:
    name: Test
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-node@v4
        with:
          node-version: "22"

      - name: Instalar dependências do frontend
        run: cd frontend && npm ci

      - name: Build do frontend
        run: cd frontend && npm run build

      - uses: actions/setup-go@v5
        with:
          go-version: "1.25"
          cache: true

      - name: go vet
        run: go vet ./...

      - name: go test
        run: go test ./... -timeout 120s

      - name: Instalar govulncheck
        run: go install golang.org/x/vuln/cmd/govulncheck@latest

      - name: govulncheck
        run: govulncheck ./...
        continue-on-error: true

  publish:
    name: Build & Publish Image
    runs-on: ubuntu-latest
    needs: test
    if: github.ref == 'refs/heads/main'

    steps:
      - uses: actions/checkout@v4

      - name: Set up Docker Buildx
        uses: docker/setup-buildx-action@v3

      - name: Login no GHCR
        uses: docker/login-action@v3
        with:
          registry: ${{ env.REGISTRY }}
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - name: Extrair metadados
        id: meta
        uses: docker/metadata-action@v5
        with:
          images: ${{ env.REGISTRY }}/${{ env.IMAGE_NAME }}
          tags: |
            type=raw,value=latest
            type=sha,prefix=sha-,format=short

      - name: Build e push
        uses: docker/build-push-action@v5
        with:
          context: .
          platforms: linux/amd64
          push: true
          tags: ${{ steps.meta.outputs.tags }}
          labels: ${{ steps.meta.outputs.labels }}
          cache-from: type=gha
          cache-to: type=gha,mode=max

      - name: Scan de vulnerabilidades (Trivy)
        uses: aquasecurity/trivy-action@master
        with:
          image-ref: ${{ env.REGISTRY }}/${{ env.IMAGE_NAME }}:latest
          format: table
          exit-code: "1"
          severity: CRITICAL,HIGH
```

## Dependabot — `.github/dependabot.yml`

Quatro ecossistemas, todos com verificação semanal: `gomod` (raiz do repositório), `npm` (`/frontend`, onde vive o `package.json`), `docker` (raiz, para a imagem base do `Dockerfile`) e `github-actions` (raiz, para as actions usadas no workflow acima).

```yaml
version: 2
updates:
  - package-ecosystem: "gomod"
    directory: "/"
    schedule:
      interval: "weekly"

  - package-ecosystem: "npm"
    directory: "/frontend"
    schedule:
      interval: "weekly"

  - package-ecosystem: "docker"
    directory: "/"
    schedule:
      interval: "weekly"

  - package-ecosystem: "github-actions"
    directory: "/"
    schedule:
      interval: "weekly"
```

## Variáveis de ambiente

Lista final e fechada — nenhuma variável fora desta tabela é lida pelo binário.

| Variável | Obrigatória | Default | Significado |
|---|---|---|---|
| `ADMIN_PASSWORD` | Sim | — | Senha única do usuário administrador; comparada em tempo constante ([Segurança](08-seguranca.md)). |
| `DB_PATH` | Sim | — | Caminho do arquivo SQLite. |
| `FILES` | Sim | — | Diretório raiz do acervo de PDFs — arquivo, thumbnail e preview ([Storage](03-storage.md)). |
| `LISTEN_ADDR` | Não | `:8000` | Endereço e porta em que o servidor HTTP escuta. |
| `BASE_URL` | Não | derivado da requisição | URL pública usada para montar links absolutos (ex.: link de compartilhamento). |
| `SESSION_IDLE_MINUTES` | Não | `43200` | Minutos de inatividade até a sessão expirar (30 dias). |
| `MAX_UPLOAD_MB` | Não | `200` | Tamanho máximo de upload, em MB, aplicado via `http.MaxBytesReader`. |
| `TRUST_PROXY_HEADERS` | Não | `false` | Se `true`, confia nos cabeçalhos `X-Forwarded-*` (IP/proto) recebidos de um proxy reverso. |
| `LOG_LEVEL` | Não | `info` | Nível de log do servidor. |
| `GEMINI_API_KEY` | Não | *(vazio)* | Chave da API Gemini. Sem ela, a busca semântica fica desligada e o botão de embedding aparece desabilitado ([Busca Híbrida](04-busca-hibrida.md)). |
| `CONSUME_ENABLE` | Não | `false` | Habilita a watch-dir de consumo automático. |
| `CONSUME_DIR` | Não | `<FILES>/consume` | Diretório observado para importação automática de PDFs. |
| `CONSUME_INTERVAL_MINUTES` | Não | `5` | Intervalo, em minutos, do ticker que varre `CONSUME_DIR`. |
| `CONSUME_TAGS` | Não | `""` | Tags aplicadas automaticamente aos PDFs importados pela watch-dir. |
| `CONSUME_SKIP_EXISTING` | Não | `true` | Pula (não importa) um arquivo cujo hash já exista no acervo. |

O modelo de embedding não é uma variável de ambiente: é a constante `config.EmbedModel = "models/gemini-embedding-2"`, fixa no código — trocá-lo exige um commit, porque muda o `content_hash` e invalida todo vetor já gravado. Mecânica completa da busca semântica sob demanda em [Busca Híbrida](04-busca-hibrida.md).

## Variáveis eliminadas

Variáveis do produto Django atual que **não têm equivalente** na versão Go — não devem ser lidas por nenhum handler, nenhum `config.go`, nenhum `docker-compose`, nenhuma etapa de `ETAPAS.md`.

| Variável | Motivo da eliminação |
|---|---|
| `DEFAULT_THEME` | Eliminada por exigência do briefing: a paleta de tema deixa de ser configurável por variável de ambiente. |
| `DEFAULT_THEME_COLOR` | Eliminada por exigência do briefing: não existe mais cor de tema configurável — paleta única fixada no `tailwind.config.ts` ([Frontend](06-frontend.md)). |
| `ACCOUNT_EMAIL_VERIFICATION` | Eliminada por exigência do briefing: não existe mais fluxo de e-mail nem verificação de conta. |
| `DISABLE_USER_SIGNUP` | Eliminada por exigência do briefing: não existe mais cadastro de usuários — o produto é single-user. |
| `ADMIN_EMAIL` | Não há e-mail no produto; resta apenas `ADMIN_PASSWORD`. |
| `SECRET_KEY` | Sessão própria (token de `crypto/rand` gravado em `sessions`) não usa uma chave de assinatura global como o Django; não há equivalente a configurar. |
| `HOST_NAME` | Headers de segurança passam a ser fixos no código ([Segurança](08-seguranca.md)); `BASE_URL` deriva da requisição quando necessário. |
| `CSRF_COOKIE_SECURE` | Atributos do cookie `csrf` (double-submit) são fixos no código, não configuráveis. |
| `SESSION_COOKIE_SECURE` | Atributos do cookie de sessão (`HttpOnly; Secure; SameSite=Lax; Path=/`) são fixos no código. |
| `SECURE_SSL_REDIRECT` | Redirecionamento e headers de transporte são fixos no middleware, não configuráveis por variável. |
| `SECURE_HSTS_SECONDS` | `Strict-Transport-Security` é um header fixo no middleware ([Segurança](08-seguranca.md)), valor não configurável. |
| `DATABASE_TYPE` | Só SQLite — não existe mais escolha de banco de dados. |
| `POSTGRES_*` | Só SQLite — PostgreSQL deixa de ser suportado. |
| `DATA_DIR` | Substituída pelo par `DB_PATH` + `FILES`, que separam explicitamente banco e acervo de arquivos. |
| `HOST_PORT` | Substituída por `LISTEN_ADDR`. |
| `WORKER_TIMEOUT` | Substituída por `LISTEN_ADDR`; não há gunicorn nem workers para configurar timeout. |
| `ALLOW_PDF_SUB_DIRECTORIES` | Deixa de ser um interruptor — subdiretórios passam a ser sempre permitidos. |
| `E2E_TESTS` | Sem equivalente no novo stack de testes/CI (ver job `test` do workflow acima). |
| `BACKUP_ENABLE`, `BACKUP_BUCKET`, `BACKUP_ENDPOINT`, `BACKUP_REGION`, `BACKUP_ACCESS_KEY_ID`, `BACKUP_SECRET_ACCESS_KEY`, `BACKUP_INTERVAL_HOURS`, `BACKUP_ENCRYPTION_ENABLE`, `BACKUP_ENCRYPTION_PASSWORD`, `BACKUP_SECURE`, `BACKUP_ENCRYPTION_SALT` | O job de backup automático para nuvem (S3/MinIO) foi **removido do escopo desta refatoração** — decisão do usuário de eliminar toda integração com Amazon S3. Backup externo, se necessário, é responsabilidade do operador do host (ver [Storage](03-storage.md)). |

**Não existe nenhuma variável `S3_*` nem `BACKUP_*`.** Os arquivos do acervo residem exclusivamente no filesystem local, sob `FILES` (ver [Storage](03-storage.md)). O produto **não tem nenhuma integração com Amazon S3, MinIO ou qualquer outro object storage** — o job de backup para nuvem que existia na versão atual foi removido nesta refatoração (ver [Funcionalidades intencionalmente removidas](10-inventario-funcionalidades.md)).

## `.env.example`

Placeholders genéricos em todo campo sensível ou específico de ambiente — nunca um valor real. O arquivo `.env` (com valores de produção) fica listado no `.gitignore` do repositório; apenas `.env.example` é versionado.

```env
# ─── Obrigatórias ────────────────────────────────────────────────────────────
ADMIN_PASSWORD=YOUR_ADMIN_PASSWORD
DB_PATH=<DB_PATH>
FILES=<FILES>

# ─── Servidor ─────────────────────────────────────────────────────────────────
LISTEN_ADDR=:8000
BASE_URL=
SESSION_IDLE_MINUTES=43200
MAX_UPLOAD_MB=200
TRUST_PROXY_HEADERS=false
LOG_LEVEL=info

# ─── Busca semântica (opcional — vazio desliga a busca semântica) ────────────
GEMINI_API_KEY=
# O modelo de embedding é fixo no binário (models/gemini-embedding-2); trocá-lo exige
# recompilar, porque muda o content_hash e invalidaria todo vetor já gravado. O modelo
# de descrição/sugestão de tags é escolhido pela interface (Configurações → IA).

# ─── Consumo automático via watch-dir (opcional) ──────────────────────────────
CONSUME_ENABLE=false
CONSUME_DIR=<FILES>/consume
CONSUME_INTERVAL_MINUTES=5
CONSUME_TAGS=
CONSUME_SKIP_EXISTING=true

```

## `compose.yaml`

Compose de desenvolvimento/self-host de instância única, na raiz do repositório — substitui os antigos `compose/postgres.docker-compose.yaml` e `compose/sqlite.docker-compose.yaml` do stack Django (ver [Limpeza do repositório](09-limpeza-repositorio.md)); não há mais escolha de banco, só SQLite. `docker compose up --build` sobe o serviço a partir do `Dockerfile` local, lendo `.env` (copiado de `.env.example`) e persistindo banco e acervo em volumes nomeados.

```yaml
services:
  newpdfding:
    build: .
    image: ghcr.io/edalcin/newpdfding:latest
    restart: unless-stopped
    read_only: true
    cap_drop:
      - ALL
    mem_limit: 1g  # limite de memória: um processo descontrolado vira restart do container, não crash do host
    ports:
      - "8000:8000"
    env_file:
      - .env
    environment:
      DB_PATH: /data/newpdfding.db
      FILES: /files
    volumes:
      - newpdfding-data:/data
      - newpdfding-files:/files

volumes:
  newpdfding-data:
  newpdfding-files:
```

## Conteúdo de UNRAID.md

`UNRAID.md` é um arquivo próprio na raiz do repositório (ver árvore em [Arquitetura](01-arquitetura.md)), não dentro de `refatoracao/`. Este documento é o dono do seu conteúdo alvo — a ETAPA-11 copia a seção abaixo quase literalmente para lá.

### Docker → Add Container

| Campo | Valor | Observação |
|---|---|---|
| Repository | `ghcr.io/edalcin/newpdfding:latest` | Imagem publicada pelo workflow de CI a cada push em `main`. |
| Network Type | `bridge` | Padrão do Unraid; não requer rede customizada. |
| Port | Container `8000` → Host `8000` (ou outra porta livre do host) | Porta HTTP do binário — variável `LISTEN_ADDR`, default `:8000`. |
| Path | Container `/data` → Host `/mnt/user/appdata/newpdfding` | Contém o arquivo SQLite (`DB_PATH`). Precisa ser gravável pelo container. |
| Path | Container `/files` → Host `<share de PDFs escolhido pelo usuário>` | Acervo de PDFs (`FILES`). Precisa ser gravável pelo container. |
| Path | Container `/files/tmp` → Host `/mnt/user/Storage/appsdata/newpdfding/temp` | Diretório temporário usado pela extração de texto de PDF. Precisa ficar num volume de disco: no Unraid, `/tmp` é RAM, e gravar ali um arquivo temporário de um PDF grande esgotaria a memória do host. Precisa ser gravável pelo container. |
| Extra Parameters | `--read-only --cap-drop=ALL --memory=1g` | `--read-only --cap-drop=ALL`: a imagem é distroless e só escreve nos volumes `/data`, `/files` e `/files/tmp` acima. `--memory=1g`: limite de memória do container — um processo descontrolado vira reinício do container, não um travamento do host. |

### Variáveis de ambiente obrigatórias

| Variável | Valor sugerido |
|---|---|
| `DB_PATH` | `/data/newpdfding.db` |
| `FILES` | `/files` |
| `ADMIN_PASSWORD` | Senha escolhida pelo usuário — nunca deixar em branco. |

### Bloco opcional — busca semântica (Gemini)

| Variável | Valor sugerido |
|---|---|
| `GEMINI_API_KEY` | Chave da API Gemini — sem ela, a busca semântica fica desligada e o botão de embedding aparece desabilitado. |

O modelo de embedding usado pela busca semântica é fixo no binário (`models/gemini-embedding-2`) — não existe variável de ambiente para trocá-lo; mudar o modelo exige um novo build.

### Troubleshooting

- **Porta ocupada**: se `8000` já estiver em uso no host, mudar apenas o lado do Host no mapeamento de porta (ex.: `8080:8000`); o container continua escutando em `8000` internamente, sem precisar mudar `LISTEN_ADDR`.
- **Permissões do share**: o processo roda como usuário não-root (uid `65532`, imagem distroless `nonroot`); os shares do host mapeados em `/data` e `/files` precisam conceder permissão de escrita a esse uid, ou o container falha ao abrir o SQLite e ao gravar PDFs.
- **Container reiniciando em loop**: checar o log do container primeiro. As causas mais comuns são `ADMIN_PASSWORD` ausente (variável obrigatória, sem default) ou `DB_PATH`/`FILES` apontando para um caminho não gravável dentro dos volumes montados.
