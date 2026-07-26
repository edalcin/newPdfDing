# newPdfDing

Gerenciador de PDFs self-hosted, single-user: biblioteca com busca híbrida (léxica + semântica sob demanda), destaques, comentários, assinaturas, coleções, tags e compartilhamento público. Backend em Go (binário único, SQLite), frontend em SvelteKit embutido no mesmo binário.

## Rodando com Docker (recomendado)

```bash
docker run -d \
  --name newpdfding \
  --read-only \
  --cap-drop=ALL \
  -p 8000:8000 \
  -e ADMIN_PASSWORD=change-me \
  -e DB_PATH=/data/newpdfding.db \
  -e FILES=/files \
  -v newpdfding-data:/data \
  -v newpdfding-files:/files \
  ghcr.io/edalcin/newpdfding:latest
```

`--read-only --cap-drop=ALL` é a execução recomendada (ver [`refatoracao/08-seguranca.md`](refatoracao/08-seguranca.md#menor-privilégio)): a imagem é distroless e não escreve em lugar nenhum além dos volumes `/data` e `/files` montados acima, então travar o resto do filesystem como somente-leitura e derrubar todas as capabilities Linux não quebra nada.

Acesse `http://localhost:8000` e entre com a senha definida em `ADMIN_PASSWORD`.

### Docker Compose

```bash
cp .env.example .env   # preencher ADMIN_PASSWORD e ajustar o resto
docker compose up --build
```

### Unraid

Guia dedicado com os campos exatos de **Docker → Add Container**: [`UNRAID.md`](UNRAID.md).

## Variáveis de ambiente

Lista fechada — nenhuma variável fora dela é lida pelo binário. Ver [`.env.example`](.env.example) para os placeholders e [`refatoracao/07-docker-ci-deploy.md`](refatoracao/07-docker-ci-deploy.md#variáveis-de-ambiente) para a tabela completa (obrigatória/default/significado).

| Variável | Obrigatória | Default |
|---|---|---|
| `ADMIN_PASSWORD` | Sim | — |
| `DB_PATH` | Sim | — |
| `FILES` | Sim | — |
| `LISTEN_ADDR` | Não | `:8000` |
| `BASE_URL` | Não | derivado da requisição |
| `SESSION_IDLE_MINUTES` | Não | `43200` |
| `MAX_UPLOAD_MB` | Não | `200` |
| `TRUST_PROXY_HEADERS` | Não | `false` |
| `LOG_LEVEL` | Não | `info` |
| `GEMINI_API_KEY` | Não | *(vazio — desliga a busca semântica)* |
| `EMBED_MODEL` | Não | `models/gemini-embedding-001` |
| `CONSUME_ENABLE` | Não | `false` |
| `CONSUME_DIR` | Não | `<FILES>/consume` |
| `CONSUME_INTERVAL_MINUTES` | Não | `5` |
| `CONSUME_TAGS` | Não | `""` |
| `CONSUME_SKIP_EXISTING` | Não | `true` |

## Desenvolvimento

Requer Go 1.25+ e Node 22+.

```bash
# Backend
go build ./cmd/newpdfding
go test ./...

# Frontend (gera frontend/build, embutido via go:embed em internal/server/web/dist)
cd frontend
npm ci
npm run build
```

Servidor local:

```bash
export ADMIN_PASSWORD=dev DB_PATH=./dev.db FILES=./dev-files
go run ./cmd/newpdfding
```

## Arquitetura

Binário único: Go (chi, SQLite puro-Go, FTS5) servindo uma API REST e a SPA SvelteKit embutida via `go:embed`. Busca híbrida (FTS5 + embeddings Gemini sob demanda, fusão RRF), sem worker nem automação de embedding. Detalhes completos do plano de refatoração e da arquitetura alvo: [`refatoracao/`](refatoracao/README.md).

## Atribuição

Este projeto é derivado de **[PdfDing](https://github.com/mrmn2/PdfDing)** por [mrmn2](https://github.com/mrmn2), licenciado sob AGPL-3.0 — ver [`LICENSE`](./LICENSE).
