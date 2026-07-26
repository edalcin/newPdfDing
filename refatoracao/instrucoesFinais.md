# Instruções finais — migração do container Unraid (Django antigo → Go novo)

Este documento cobre a atualização do container Unraid existente (criado para o stack Django antigo) para a imagem nova `ghcr.io/edalcin/newpdfding:latest` (Go + SvelteKit), incluindo a migração dos dados via `-import-legacy` (ver [ETAPAS.md](ETAPAS.md), ETAPA-12-IMPORTACAO).

O template atual do container tem várias variáveis herdadas do Django que o binário Go novo ignora, e os dois `Path` apontam para dentro de um container que não existe mais nessa forma (a imagem nova é `distroless`, sem `/home/nonroot/`).

## 1. Apagar (variáveis Django que o binário novo ignora)

| Remover | Por quê |
|---|---|
| `HOST_NAME` | Django `ALLOWED_HOSTS`. App novo não valida Host. |
| `SECRET_KEY` | Django `SECRET_KEY`. Sessão nova usa `crypto/rand`, sem segredo compartilhado. |
| `CSRF_COOKIE_SECURE` | Cookies (`csrf`, sessão) são `Secure` fixo no código, não configurável. |
| `SESSION_COOKIE_SECURE` | Idem. |
| `HOST_PORT` | Não lido; o mapeamento de porta já é feito pelo campo `webUI` do Unraid. |
| `DATABASE_TYPE` | Só existe SQLite agora, sem Postgres. |
| `DEFAULT_THEME` / `DEFAULT_THEME_COLOR` | Tema é preferência de UI (`settings` no banco), não variável de ambiente. |
| `ACCOUNT_EMAIL_VERIFICATION` | Sem conta de usuário/e-mail no produto novo. |
| `DISABLE_USER_SIGNUP` | Sem multi-usuário/signup. |
| `ADMIN_EMAIL` | Sem login por e-mail — só senha. |

## 2. Alterar (os dois `Path`)

A imagem nova espera exatamente `/data` (arquivo SQLite) e `/files` (acervo de PDFs) como container path — é o que o `Dockerfile`/`compose.yaml` do projeto fixam.

| Campo | Host Path | Container Path — trocar para |
|---|---|---|
| `db` | mantenha, mas **use uma pasta nova e vazia**, ex.: `/mnt/user/Storage/appsdata/newpdfding/db` | `/data` |
| `media` | idem, ex.: `/mnt/user/Storage/appsdata/newpdfding/files` | `/files` |

**Não aponte direto para as pastas antigas `pdfding/db` e `pdfding/media`** — o schema SQLite e o layout de arquivos mudaram completamente (ver seção 4, migração).

## 3. Manter / Adicionar

| Variável | Valor | Observação |
|---|---|---|
| `ADMIN_PASSWORD` | mantenha o valor atual (ou troque) | única credencial agora, sem e-mail associado |
| `DB_PATH` *(nova, adicionar)* | `/data/newpdfding.db` | obrigatória |
| `FILES` *(nova, adicionar)* | `/files` | obrigatória |
| `TRUST_PROXY_HEADERS` *(nova, recomendada)* | `true` | se o domínio (`newpdfding.dalc.in`) fica atrás de um proxy reverso, isso faz o rate-limit de login usar o IP real do `X-Forwarded-For`, não o IP do proxy |
| `BASE_URL` *(nova, recomendada)* | `https://newpdfding.dalc.in` | garante que os links de compartilhamento público saiam com o domínio certo mesmo atrás de proxy |

O campo `webUI` (ex.: `8778` → container `8000`) e `Console shell command: Shell` podem ficar como estão — só que o botão **Console** do Unraid vai falhar se usado (a imagem é `distroless`, não tem shell). Isso é esperado, não é erro de configuração.

## 4. Migração dos dados antigos (schema incompatível — precisa do import)

Apontar `DB_PATH`/`FILES` para pastas vazias novas faz o app subir limpo, **sem os PDFs já cadastrados**. Para trazê-los, existe um comando único de importação (`-import-legacy`) feito exatamente para isso. Rode isso **uma vez**, via terminal do Unraid (não pelo Console do container — ele não tem shell):

```bash
# 1) Criar as pastas novas e liberar escrita para o usuário do container (uid 65532)
mkdir -p /mnt/user/Storage/appsdata/newpdfding/db /mnt/user/Storage/appsdata/newpdfding/files
chmod -R 777 /mnt/user/Storage/appsdata/newpdfding

# 2) Rodar o import — lê o banco/mídia Django antigos, grava direto nas pastas novas
docker run --rm \
  -e ADMIN_PASSWORD='<mesma senha do ADMIN_PASSWORD do container>' \
  -e DB_PATH=/data/newpdfding.db \
  -e FILES=/files \
  -v /mnt/user/Storage/appsdata/newpdfding/db:/data \
  -v /mnt/user/Storage/appsdata/newpdfding/files:/files \
  -v /mnt/user/Storage/appsdata/pdfding/db:/legacy-db:ro \
  -v /mnt/user/Storage/appsdata/pdfding/media:/legacy-media:ro \
  ghcr.io/edalcin/newpdfding:latest \
  -import-legacy /legacy-db/db.sqlite3 /legacy-media
```

O log final mostra quantas coleções/tags/PDFs/anotações/shares foram importados (o caminho `db.sqlite3` foi confirmado em `settings/base.py` do projeto Django original — é exatamente `<pasta db>/db.sqlite3`).

Depois disso, **inicie o container normal** (Update Container com os campos das seções 2 e 3) apontando para as mesmas pastas novas `/mnt/user/Storage/appsdata/newpdfding/db` e `/files` — os dados já estarão lá.

## 5. Endurecimento opcional (depois de confirmar que funciona)

Em "Show more settings…" tem um campo de parâmetros extras do `docker run`; adicionar `--read-only --cap-drop=ALL` é a recomendação de segurança do projeto (imagem só escreve em `/data`/`/files`, o resto pode ficar travado — ver [08-seguranca.md](08-seguranca.md), "Menor privilégio"). Faça isso só depois de validar o container rodando normalmente, para isolar qualquer problema de permissão.
