# newPdfDing no Unraid

Guia de instalação manual via **Docker → Add Container** no Unraid. Não há template no Community Applications — configure os campos abaixo diretamente.

## Docker → Add Container

| Campo | Valor | Observação |
|---|---|---|
| Repository | `ghcr.io/edalcin/newpdfding:latest` | Imagem publicada pelo workflow de CI a cada push em `main`. |
| Network Type | `bridge` | Padrão do Unraid; não requer rede customizada. |
| Port | Container `8000` → Host `8000` (ou outra porta livre do host) | Porta HTTP do binário — variável `LISTEN_ADDR`, default `:8000`. |
| Path | Container `/data` → Host `/mnt/user/appdata/newpdfding` | Contém o arquivo SQLite (`DB_PATH`). Precisa ser gravável pelo container. |
| Path | Container `/files` → Host `<share de PDFs escolhido pelo usuário>` | Acervo de PDFs (`FILES`). Precisa ser gravável pelo container. |

## Variáveis de ambiente obrigatórias

| Variável | Valor sugerido |
|---|---|
| `DB_PATH` | `/data/newpdfding.db` |
| `FILES` | `/files` |
| `ADMIN_PASSWORD` | Senha escolhida pelo usuário — nunca deixar em branco. |

## Bloco opcional — busca semântica (Gemini)

| Variável | Valor sugerido |
|---|---|
| `GEMINI_API_KEY` | Chave da API Gemini — sem ela, a busca semântica fica desligada e o botão de embedding aparece desabilitado. |
| `EMBED_MODEL` | `models/gemini-embedding-001` (default; normalmente não precisa mudar). |

Lista completa de variáveis (incluindo consumo automático via watch-dir): ver [`.env.example`](.env.example) e [`refatoracao/07-docker-ci-deploy.md`](refatoracao/07-docker-ci-deploy.md).

## Troubleshooting

- **Porta ocupada**: se `8000` já estiver em uso no host, mudar apenas o lado do Host no mapeamento de porta (ex.: `8080:8000`); o container continua escutando em `8000` internamente, sem precisar mudar `LISTEN_ADDR`.
- **Permissões do share**: o processo roda como usuário não-root (uid `65532`, imagem distroless `nonroot`); os shares do host mapeados em `/data` e `/files` precisam conceder permissão de escrita a esse uid, ou o container falha ao abrir o SQLite e ao gravar PDFs.
- **Container reiniciando em loop**: checar o log do container primeiro. As causas mais comuns são `ADMIN_PASSWORD` ausente (variável obrigatória, sem default) ou `DB_PATH`/`FILES` apontando para um caminho não gravável dentro dos volumes montados.
