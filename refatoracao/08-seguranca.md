# 08 — Segurança

Este documento fixa todos os mecanismos de segurança do backend Go: autenticação, rate limiting, CSRF, headers HTTP, validação de entrada, princípio de menor privilégio, gestão de segredos e verificações de CI. Nenhum item aqui é opcional ou configurável por variável de ambiente, salvo onde explicitamente indicado.

Para a lista completa de variáveis de ambiente (incluindo `ADMIN_PASSWORD`, `SESSION_IDLE_MINUTES`, `MAX_UPLOAD_MB`) e o workflow de CI, ver [07-docker-ci-deploy.md](07-docker-ci-deploy.md). Para o contrato de rotas citado abaixo, ver [05-api.md](05-api.md). Para a validação de chaves de armazenamento (path traversal em `storage.Backend`), ver [03-storage.md](03-storage.md).

## Autenticação

- **Usuário único, senha única**: a aplicação não tem tabela de usuários. A credencial é a variável de ambiente `ADMIN_PASSWORD` (obrigatória — ver [07-docker-ci-deploy.md](07-docker-ci-deploy.md)).
- **Comparação em tempo constante**: a senha enviada em `POST /api/auth/login` é comparada com `ADMIN_PASSWORD` via `subtle.ConstantTimeCompare`, nunca com `==` ou comparação de string padrão, para eliminar ataques de timing.
- **Sessão**:
  - Token de **32 bytes** gerados por `crypto/rand`, codificados em **base64url**.
  - Gravado na tabela `sessions` (ver [02-modelo-de-dados.md](02-modelo-de-dados.md)), colunas `id`, `created_at`, `last_seen_at`.
  - Entregue ao navegador em cookie com atributos:
    - `HttpOnly`
    - `Secure`
    - `SameSite=Lax`
    - `Path=/`
- **Expiração**: uma sessão expira quando `now - last_seen_at > SESSION_IDLE_MINUTES` (default `43200`, ou seja, 30 dias — ver [07-docker-ci-deploy.md](07-docker-ci-deploy.md)). Cada requisição autenticada atualiza `last_seen_at`.
- **Limpeza**: uma rotina apaga sessões expiradas da tabela `sessions` a cada hora.

## Rate limiting

- **5 tentativas de login falhas** a partir do mesmo IP disparam **bloqueio de 30 minutos** para esse IP em `POST /api/auth/login`.
- Contador mantido **em memória** (não persistido em SQLite); reinicia quando o processo reinicia.
- Isto é uma limitação conhecida e aceita — documentada aqui como tal, não como lacuna a corrigir: um restart do container zera os contadores de bloqueio. Não requer nenhuma ação adicional.

## CSRF

- Estratégia **double-submit cookie**:
  - Cookie `csrf`, **não** `HttpOnly` (precisa ser legível por JavaScript no frontend).
  - O frontend lê o cookie e o reenvia no header `X-CSRF-Token` em toda requisição.
  - O servidor exige que o header bata com o cookie em **todo método não-idempotente** (`POST`, `PATCH`, `PUT`, `DELETE`).
- **Isenção**: as rotas públicas de compartilhamento (`GET /s/{share_id}`, `GET /api/shared/{share_id}`, `GET /api/shared/{share_id}/file` — ver [05-api.md](05-api.md)) são somente-leitura e não exigem CSRF.

## Headers de segurança

Todos os headers abaixo são **fixos no middleware do servidor Go**, escritos em toda resposta HTTP. Não existe variável de ambiente para nenhum deles — não são configuráveis, são política do produto.

| Header | Valor |
|---|---|
| `Strict-Transport-Security` | `max-age=31536000; includeSubDomains` |
| `X-Content-Type-Options` | `nosniff` |
| `X-Frame-Options` | `SAMEORIGIN` |
| `Referrer-Policy` | `no-referrer` |
| `Permissions-Policy` | `geolocation=(), microphone=(), camera=()` |
| `Content-Security-Policy` | ver bloco abaixo |

`X-Frame-Options: SAMEORIGIN` (em vez de `DENY`) porque o viewer usa um `<iframe>` do pdf.js servido pelo mesmo domínio (ver [06-frontend.md](06-frontend.md)).

### Content-Security-Policy — string literal completa

```
default-src 'self'; img-src 'self' data: blob:; script-src 'self' 'wasm-unsafe-eval'; worker-src 'self' blob:; style-src 'self' 'unsafe-inline'; connect-src 'self'; frame-ancestors 'self'; object-src 'none'; base-uri 'none'
```

Justificativa de cada relaxamento, nenhum é gratuito:

- **`script-src 'self' 'wasm-unsafe-eval'`**: o motor PDFium do EmbedPDF (ver [06-frontend.md](06-frontend.md)) compila e executa WebAssembly para decodificação de PDF; `'wasm-unsafe-eval'` é exigido pelo navegador para instanciar esse módulo Wasm. **Não** há `'unsafe-inline'` em `script-src` — nenhum script inline é permitido, elimina a classe mais comum de XSS.
- **`worker-src 'self' blob:`**: o EmbedPDF instancia seus workers (motor PDFium e encoder de imagem) a partir de `blob:` URLs via `URL.createObjectURL` — sem esta diretiva o worker é bloqueado e o SDK precisa rodar na thread principal, o que trava a aba em PDFs grandes (ver [11-desempenho-viewer.md](11-desempenho-viewer.md), causa C1). Um worker criado a partir de `blob:` roda em contexto isolado sem acesso ao DOM, então esta diretiva é estritamente mais restrita que permitir `blob:` em `script-src` seria — não abre execução de script na página.
- **`img-src ... blob:`**: as páginas renderizadas pelo pdf.js e as thumbnails/previews geradas no navegador (canvas → PNG) são exibidas via `blob:` URLs antes do upload.
- **`style-src 'self' 'unsafe-inline'`**: componentes shadcn-svelte e utilitários gerados pelo Tailwind aplicam estilos inline em alguns pontos de runtime; `'unsafe-inline'` fica restrito a `style-src`, onde o impacto de segurança é muito menor que em `script-src` (não permite execução de código).
- **`object-src 'none'`** e **`base-uri 'none'`**: fecham os dois vetores clássicos de bypass de CSP (plugins embutidos e reescrita de `<base>`), sem custo funcional — o produto não usa nenhum dos dois.
- **`frame-ancestors 'self'`**: reforça `X-Frame-Options`, impedindo que a aplicação seja embutida em iframe de outro domínio (clickjacking).
- **`connect-src 'self'`**: todas as chamadas `fetch` do SvelteKit são para a própria API; nada de terceiros é contatado pelo browser (a chamada à API Gemini acontece no servidor, nunca no cliente).

## Validação de entrada

- **Tipo de arquivo por magic bytes**: o upload de PDF é validado lendo os primeiros bytes do próprio stream e conferindo a assinatura `%PDF-`, sem depender de `Content-Type` declarado pelo cliente. Substitui `python-magic`/`libmagic` da versão Django — elimina uma dependência nativa (C) do binário Go, mantendo `CGO_ENABLED=0`.
- **Limite de tamanho**: `MAX_UPLOAD_MB` (default `200`, ver [07-docker-ci-deploy.md](07-docker-ci-deploy.md)) é aplicado via `http.MaxBytesReader`, cortando a requisição antes de consumir memória ou disco além do limite.
- **Nomes de arquivo nunca vindos do cliente**: a chave de armazenamento em disco é sempre derivada de `pdf_id` (UUIDv7 gerado no servidor), nunca do nome de arquivo enviado no `multipart/form-data`. Ver esquema de chaves em [03-storage.md](03-storage.md).
- **SQL**: toda consulta usa parâmetros vinculados (`database/sql` com placeholders `?`), nunca concatenação de string. Nenhuma exceção em nenhum handler.
- **Notas em Markdown**: o campo `notes` (Markdown bruto, ver [02-modelo-de-dados.md](02-modelo-de-dados.md)) é renderizado para HTML por `github.com/yuin/goldmark` e em seguida sanitizado por `bluemonday.UGCPolicy()` **antes** de sair em qualquer resposta JSON — o cliente nunca recebe HTML não sanitizado, independente de o navegador confiar ou não em CSP.

## Menor privilégio

- Imagem final `gcr.io/distroless/static-debian12:nonroot` (ver [07-docker-ci-deploy.md](07-docker-ci-deploy.md)):
  - Processo roda como `USER nonroot`, **uid 65532**.
  - Sem shell, sem gerenciador de pacotes, sem qualquer binário além do próprio `newpdfding` — não há superfície para `exec` pós-comprometimento.
  - Filesystem da imagem é somente leitura por natureza (distroless); os únicos caminhos graváveis são os volumes montados (`DB_PATH`, `FILES`).
- **Recomendação de execução**, a incluir nas instruções de deploy ([07-docker-ci-deploy.md](07-docker-ci-deploy.md) e `UNRAID.md`): rodar o container com `--read-only --cap-drop=ALL`, montando apenas os volumes de dados como graváveis.

## Segredos

- **Nada embutido na imagem**: nenhuma credencial, chave ou senha é copiada para a imagem Docker em nenhum estágio do build. Todas as credenciais (`ADMIN_PASSWORD`, `GEMINI_API_KEY`) chegam exclusivamente por variável de ambiente em tempo de execução.
- **`.env` fora do git**: o arquivo `.env` real (com valores de produção) está no `.gitignore`; apenas `.env.example`, com placeholders genéricos, é versionado (ver [07-docker-ci-deploy.md](07-docker-ci-deploy.md)).

## CI

- **`govulncheck`** roda no job `test` do workflow (ver [07-docker-ci-deploy.md](07-docker-ci-deploy.md)), verificando vulnerabilidades conhecidas nas dependências Go antes de qualquer build de imagem.
- **Trivy** roda após o `publish`, com `exit-code: 1` para severidade `CRITICAL,HIGH` — falha o pipeline, não apenas reporta (mais rígido que a referência `pkd`, que só reporta sem falhar).
- **Dependabot** semanal para os quatro ecossistemas do projeto: `gomod`, `npm`, `docker`, `github-actions` (ver `.github/dependabot.yml` em [07-docker-ci-deploy.md](07-docker-ci-deploy.md)).

## Checklist final

- [x] Login compara senha com `subtle.ConstantTimeCompare`.
- [x] Sessão usa token de 32 bytes de `crypto/rand` em base64url, cookie `HttpOnly; Secure; SameSite=Lax; Path=/`.
- [x] Sessões expiram por `last_seen_at + SESSION_IDLE_MINUTES` e são limpas a cada hora.
- [x] Rate limit de login: 5 falhas → bloqueio de 30 minutos por IP.
- [x] CSRF double-submit (cookie `csrf` + header `X-CSRF-Token`) exigido em todo método não-idempotente, exceto rotas públicas de compartilhamento.
- [x] Todos os headers fixos aplicados em toda resposta: HSTS, `X-Content-Type-Options`, `X-Frame-Options`, `Referrer-Policy`, `Permissions-Policy`, CSP.
- [x] CSP contém exatamente a string literal especificada, sem `'unsafe-inline'` em `script-src` (mais um hash `sha256-` do bootstrap estático do SvelteKit, computado em build — ver `internal/server/csp.go`; nenhum `'unsafe-inline'` é adicionado).
- [x] Upload valida magic bytes `%PDF-` no stream, não confia em `Content-Type` do cliente.
- [x] `MAX_UPLOAD_MB` aplicado via `http.MaxBytesReader`.
- [x] Nenhum caminho de disco é derivado de nome de arquivo enviado pelo cliente; chave sempre por `pdf_id`.
- [x] Todo SQL usa parâmetros vinculados, nunca concatenação.
- [x] Notas Markdown passam por `goldmark` + `bluemonday.UGCPolicy()` antes de qualquer resposta JSON.
- [x] Imagem final é `distroless:nonroot` (uid 65532), sem shell.
- [x] Instruções de deploy recomendam `--read-only --cap-drop=ALL` (README.md, compose.yaml, UNRAID.md).
- [x] Nenhuma credencial embutida na imagem; `.env` fora do git.
- [x] `govulncheck` no job de teste do CI.
- [x] Trivy falha o pipeline em `CRITICAL,HIGH`.
- [x] Dependabot configurado semanalmente para `gomod`, `npm`, `docker`, `github-actions`.
