# newPdfDing no Unraid

Guia de instalação manual via **Docker → Add Container** no Unraid. Não há template no Community Applications — configure os campos abaixo diretamente.

## Docker → Add Container

| Campo | Valor | Observação |
|---|---|---|
| Repository | `ghcr.io/edalcin/newpdfding:latest` | Imagem publicada pelo workflow de CI a cada push em `main`. |
| Network Type | `bridge` | Padrão do Unraid; não requer rede customizada. |
| Port | Container `8000` → Host `8000` (ou outra porta livre do host) | Porta HTTP do binário — variável `LISTEN_ADDR`, default `:8000`. |
| Path | Container `/data` → Host `/mnt/user/appdata/newpdfding/db` | Contém o arquivo SQLite (`DB_PATH`). Precisa ser gravável pelo container. |
| Path | Container `/files` → Host `/mnt/user/appdata/newpdfding/files` | Acervo de PDFs (`FILES`). Precisa ser gravável pelo container. |
| Path | Container `/files/tmp` → Host `/mnt/user/appdata/newpdfding/temp` | Diretório temporário usado pela extração de texto de PDF. Precisa ficar num volume de disco: no Unraid, `/tmp` é RAM, e gravar ali um arquivo temporário de um PDF grande esgotaria a memória do host. Precisa ser gravável pelo container. |
| Extra Parameters | `--read-only --cap-drop=ALL --memory=1g` | `--read-only --cap-drop=ALL` (ver [`refatoracao/08-seguranca.md`](refatoracao/08-seguranca.md#menor-privilégio)): a imagem é distroless e só escreve nos volumes `/data`, `/files` e `/files/tmp` acima, então travar o resto do filesystem como somente-leitura e derrubar todas as capabilities Linux não quebra nada. `--memory=1g`: limite de memória do container — um processo descontrolado vira reinício do container, não um travamento do host. |

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

O modelo de embedding usado pela busca semântica é fixo no binário (`models/gemini-embedding-2`, ver [`refatoracao/04-busca-hibrida.md`](refatoracao/04-busca-hibrida.md)) — não existe variável de ambiente para trocá-lo; mudar o modelo exige um novo build.

Lista completa de variáveis (incluindo consumo automático via watch-dir): ver [`.env.example`](.env.example) e [`refatoracao/07-docker-ci-deploy.md`](refatoracao/07-docker-ci-deploy.md).

## Troubleshooting

- **Porta ocupada**: se `8000` já estiver em uso no host, mudar apenas o lado do Host no mapeamento de porta (ex.: `8080:8000`); o container continua escutando em `8000` internamente, sem precisar mudar `LISTEN_ADDR`.
- **Permissões do share**: o processo roda como usuário não-root (uid `65532`, imagem distroless `nonroot`); os shares do host mapeados em `/data` e `/files` precisam conceder permissão de escrita a esse uid, ou o container falha ao abrir o SQLite e ao gravar PDFs.
- **Container reiniciando em loop**: checar o log do container primeiro. As causas mais comuns são `ADMIN_PASSWORD` ausente (variável obrigatória, sem default) ou `DB_PATH`/`FILES` apontando para um caminho não gravável dentro dos volumes montados.
- **Aviso `kernel does not support swap limit capabilities` ao subir o container**: inofensivo quando o host não tem swap (`swapon -s` vazio, o caso do Unraid por padrão). Sem swap, `--memory=1g` é teto rígido: o cgroup mata o processo em vez de empurrá-lo para disco, que é o comportamento desejado.
- **Ajustar `--memory`**: em regime normal com um documento sendo embedado por vez, o container fica na casa de **30 MiB** (medido com `docker stats --no-stream newPdfDing`). O `1g` é folga deliberada para o pico da extração de um PDF grande; apertar para `256m` continua sendo várias vezes o regime observado, se o teto mais baixo for preferível.
