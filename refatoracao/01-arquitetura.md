# Arquitetura

Este documento descreve a árvore de diretórios alvo, as camadas do sistema e a regra de dependência entre elas, a interface de armazenamento a ser implementada, a configuração obrigatória do SQLite, o mecanismo de migração de schema e a tabela fixa de dependências Go que serve de ponto de partida para o `go.mod`.

Para o schema completo e o detalhamento da migração de dados, ver [Modelo de Dados](02-modelo-de-dados.md). Para o detalhamento da implementação de armazenamento (`LocalBackend`, esquema de chaves, backup), ver [Storage](03-storage.md).

## Árvore de diretórios alvo

Esta é a estrutura decidida — o implementador não escolhe uma estrutura alternativa:

```
cmd/newpdfding/main.go
internal/config/config.go
internal/store/            migrate.go schema.sql pdfs.go tags.go collections.go
                           annotations.go shares.go search.go semantic.go
                           settings.go sessions.go
internal/storage/          storage.go local.go
internal/server/           server.go router.go middleware.go
                           handlers_pdfs.go handlers_tags.go handlers_collections.go
                           handlers_annotations.go handlers_search.go handlers_share.go
                           handlers_settings.go handlers_admin.go handlers_auth.go
                           consumer.go backup.go
                           web/dist/           (saída do build do frontend, go:embed)
internal/security/         tokens.go headers.go ratelimit.go sanitize.go
frontend/                  src/ static/ svelte.config.js vite.config.ts
                           tailwind.config.ts components.json package.json
Dockerfile .dockerignore .env.example
.github/workflows/docker-publish.yaml .github/dependabot.yml
README.md UNRAID.md CHANGELOG.md LICENSE SECURITY.md
```

## Camadas e regra de dependência

- `server` → depende de `store` e `storage`.
- `store` nunca importa `server`.
- `storage` não conhece o domínio: opera exclusivamente em `key string`, sem saber o que é um PDF, uma coleção ou uma tag.

Justificativa: manter `store` e `storage` livres de qualquer referência a `server` (e `storage` livre de qualquer referência de domínio) garante que a camada HTTP seja a única que conhece rotas, sessões e serialização JSON — o resto do sistema permanece testável isoladamente e substituível sem tocar no protocolo HTTP.

## Interface `storage.Backend`

Assinatura exata a implementar, copiada da referência `pkd` (`internal/storage/storage.go`, somente leitura, ignorando tudo que for específico de S3 nesse arquivo):

```go
type Backend interface {
    Put(ctx context.Context, key string, body io.Reader, size int64, contentType string) error
    Get(ctx context.Context, key string) (io.ReadCloser, int64, error)
    Delete(ctx context.Context, key string) error
    List(ctx context.Context, prefix string) ([]string, error)
    Name() string // "local" | "s3"
}

type Seeker interface {
    OpenSeek(ctx context.Context, key string) (io.ReadSeekCloser, int64, error)
}
```

Neste projeto existe uma única implementação, `LocalBackend`, com raiz em `FILES`. Detalhes de esquema de chaves, validação de path traversal, entrega via `http.ServeContent`, tratamento de falhas e o job de backup estão em [Storage](03-storage.md).

## Configuração SQLite obrigatória

Idêntica a `pkd` (`internal/store/migrate.go`), aplicada na abertura da conexão:

- `db.SetMaxOpenConns(1)`
- `PRAGMA journal_mode=WAL`
- `PRAGMA synchronous=NORMAL`
- `PRAGMA foreign_keys=ON`
- `PRAGMA busy_timeout=5000`

## Mecanismo de migração

- Schema embutido via `//go:embed schema.sql`.
- Todo DDL usa `IF NOT EXISTS` — o schema completo é reaplicável a cada boot sem efeito colateral.
- Aplicado em transação única no boot.
- Migrações de coluna são um slice de `ALTER TABLE ... ADD COLUMN`, tolerando o erro `duplicate column name` (idempotência sem tabela de versão de migração).
- O índice FTS5 é reconstruído no boot (ver [Busca Híbrida](04-busca-hibrida.md) para o procedimento de rebuild).
- Sem goose, sem golang-migrate, sem qualquer framework de migração.
- Sem migrações de rollback — o schema só avança.

Detalhamento completo do `schema.sql` e do mapeamento modelo Django → tabela nova em [Modelo de Dados](02-modelo-de-dados.md).

## Dependências Go fixadas

Versões que `pkd` usa hoje — piso, não teto. Confirmar com `go get` na execução da etapa correspondente; se `go get` trouxer versão maior, usar a maior e atualizar esta tabela.

| Módulo | Versão | Uso |
|---|---|---|
| `github.com/go-chi/chi/v5` | `v5.2.5` | Router HTTP |
| `modernc.org/sqlite` | `v1.48.2` | Driver SQLite puro Go (sem CGO) |
| `github.com/google/uuid` | `v1.6.0` | UUIDv7 (`uuid.NewV7()`) para todos os IDs de entidade |
| `github.com/microcosm-cc/bluemonday` | `v1.0.27` | Sanitização de HTML (notas em Markdown) |
| `github.com/yuin/goldmark` | (piso a confirmar) | Renderização de Markdown → HTML das notas, substitui `Markdown`+`nh3` do Python |
| `github.com/ledongthuc/pdf` | (piso a confirmar) | Extração de texto pura-Go, usada apenas no caminho da watch-dir |
| `github.com/aws/aws-sdk-go-v2` | `v1.41.7` | Usado só pelo job de backup |
| `github.com/aws/aws-sdk-go-v2/config` | `v1.32.17` | Usado só pelo job de backup |
| `github.com/aws/aws-sdk-go-v2/credentials` | `v1.19.16` | Usado só pelo job de backup |
| `github.com/aws/aws-sdk-go-v2/feature/s3/manager` | `v1.22.18` | Usado só pelo job de backup |
| `github.com/aws/aws-sdk-go-v2/service/s3` | `v1.101.0` | Usado só pelo job de backup |

Nota: as cinco últimas dependências (`aws-sdk-go-v2` e submódulos) entram no `go.mod` exclusivamente pelo job de backup para S3/MinIO (ver [Storage](03-storage.md)) — não há nenhum outro uso de S3 no produto, já que os arquivos residem exclusivamente no filesystem local sob `FILES`.
