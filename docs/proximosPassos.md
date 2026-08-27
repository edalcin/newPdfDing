# Próximos passos

> Handoff de 2026-08-27, fim da segunda sessão do dia. Estado: `main` em `d184741`, árvore limpa, CI verde, imagem publicada e **em produção no UNRAID**.
> A sessão anterior deixou cinco tarefas pendentes: três foram feitas, duas foram **eliminadas por decisão** — nenhuma continua aberta.

---

## 1. Estado de produção — em operação

Imagem `ghcr.io/edalcin/newpdfding:latest` do commit `d184741` rodando no UNRAID desde 2026-08-27 ~20:55. Embedding manual, um documento por vez, **funcionando e verificado no banco**.

| | |
|---|---|
| Vetores gravados | 2 de 167 (o usuário está embedando aos poucos, de propósito) |
| Dimensões | 3072 em todos, 12.288 bytes `float32` — coerente com `gemini-embedding-2` |
| Tamanhos distintos de vetor | 1 — nenhum vetor do modelo antigo sobrou misturado |
| `content_hash` | 64 caracteres (sha256 completo) em todos |
| Órfãos em `pdf_embeddings` | 0 |
| `integrity_check` | `ok` |
| Chave `ai.embed_model` | ausente do banco e do código |
| Memória do container | **30 MiB de 1 GiB (2,94%)** |
| `/mnt/user/Storage/appsdata/newpdfding/temp` | vazio |

**Backup pré-limpeza, em dois lugares:** `newpdfding.db.bak-20260827-164727` (`VACUUM INTO`, `integrity_check ok`, md5 `32e4341e1da290c84c30c64c39ee3216`) no próprio servidor e em `D:/git/backups/`. Continha 167 PDFs, 117 vetores do modelo antigo, 124 textos extraídos, 2 anotações, 57 tags.

`pdf_text` **não** foi tocado na limpeza: texto extraído não depende de modelo, e apagá-lo forçaria 124 extrações de PDF desnecessárias — justamente o caminho que derrubou o host.

**Observação de desempenho do usuário:** `gemini-embedding-2` é perceptivelmente mais lento que `gemini-embedding-001`. O tempo é da API do Google (uma chamada HTTP por documento); não há nada a otimizar do nosso lado, e o modelo é maior e multimodal.

### Configuração real em produção

```
-v /mnt/user/Storage/appsdata/newpdfding/db:/data:rw
-v /mnt/user/Storage/appsdata/newpdfding/files:/files:rw
-v /mnt/user/Storage/appsdata/newpdfding/temp:/files/tmp:rw
--read-only --cap-drop=ALL --memory=1g
-p 8778:8000
```

`EMBED_MODEL` não está mais no template. O aviso `kernel does not support swap limit capabilities` na subida é inofensivo: o host não tem swap (`Swap: 0`), então `--memory=1g` é teto rígido.

## 2. O que mudou no código (commit `d184741`, `+243 −573`)

**Modelo fixo.** `config.EmbedModel = "models/gemini-embedding-2"`, constante. Sumiram: `EMBED_MODEL`, `cfg.EmbedModel`, a chave `ai.embed_model`, `Server.embedModelName()` e o pulldown em Configurações → IA. `/admin` exibe "Modelo de embedding (fixo): models/gemini-embedding-2", sem controle. `ai.text_model` (descrição e sugestão de tags) continua escolhível. Ver [ADR 0001](adr/0001-modelo-de-embedding-fixo-no-codigo.md).

**Sem embedding em massa.** Removidos `POST /api/admin/reembed`, `enqueueBulk`, `PendingEmbeddingIDs`, o botão "Reembedar pendentes" e o `pollInfo` de 5 s. Um documento por vez, pelo ícone de cérebro no card (`POST /api/pdfs/{id}/embed`, intocado). Ver [ADR 0002](adr/0002-sem-embedding-em-massa.md).

**Extração sai da RAM.** Temporário em `<FILES>/tmp` com `os.MkdirAll(..., 0o750)`, não mais em `/tmp` (RAM no UNRAID). `capText` limita o texto extraído a `extractedTextCapBytes = 2 MB` com `io.CopyN` + `buf.Grow`, espelhando `TEXT_LIMIT_BYTES` do navegador. `cleanOrphanedTempFiles` remove `npd-*.pdf` com mais de 6 h.

**`Stats()` para de carregar bodies inteiros.** Query única em streaming com `LEFT JOIN pdf_text` e `LEFT JOIN pdf_embeddings`, sem cláusula `IN` (que estouraria `SQLITE_MAX_VARIABLE_NUMBER` num acervo grande). O caminho paginado de 25 itens mantém o `IN` e ganhou `substr(body, 1, embedBodyChars)`. `TestAttachEmbeddingStatusHashSurvivesSQLCharTruncation` prova que o hash não mudou de valor — a invariante que, se quebrasse, marcaria o acervo todo como desatualizado.

**Limite de memória.** `mem_limit: 1g` no `compose.yaml`, `--memory=1g` no `UNRAID.md`.

## 3. Verificação: o que está provado e o que falta

Provado na imagem Docker real (`--read-only --memory=1g`) e, depois, em produção:

| Checado | Resultado |
|---|---|
| `GET /api/admin/info` | `"embed_model":"models/gemini-embedding-2"`, quatro contadores intactos |
| `POST /api/admin/reembed` | `405` — rota não existe |
| `PATCH /api/settings` com `ai.embed_model` | `400 unknown key "ai.embed_model"` |
| `/admin` | nome do modelo exibido, sem botão de lote |
| `/settings` → IA | só o modelo de texto |
| Vetores em produção | 3072 dims, hash sha256, sem órfãos, `integrity_check ok` |
| Memória em produção | 30 MiB de 1 GiB |
| CI do commit | `Test` + `Build & Publish Image` verdes (run `33114619047`) |

`go build`/`go vet`/`go test ./...` limpos; `npm run check` 0 erros/0 warnings; `npm run build` ok.

**Único ponto ainda sem prova de produção:** o caminho `extractPDFTextFromStorage` (temporário em disco). Os dois documentos embedados até agora **já tinham texto extraído**, então nem passaram por ele — o `temp` ficou vazio o tempo todo, o que é o comportamento correto, não uma falha. Cobertura atual: `TestExtractPDFTextFromStorage_TempFileUnderFilesTmp` intercepta `createTempFile` e afirma que o diretório é exatamente `<cfg.Files>/tmp` e nunca `os.TempDir()`, nos caminhos de sucesso **e** de erro, e que nada sobra depois.

**Como fechar:** embedar um dos **43 documentos sem texto extraído** (167 PDFs − 124 textos) olhando o diretório:

```bash
watch -n 2 'docker stats --no-stream --format "{{.MemUsage}} {{.MemPerc}}" newPdfDing; ls /mnt/user/Storage/appsdata/newpdfding/temp'
```

Um `npd-*.pdf` que **aparece durante** a extração e **desaparece no fim** é a prova positiva. Um que **fica** depois é defeito — um `defer os.Remove` não rodou.

## 4. Decisões desta sessão que não devem ser relitigadas

- **Sem filtro por estado de embedding na listagem.** Cogitado e recusado por não ser necessário; `/admin` já responde quantos faltam. **Gatilho para implementar:** se rolar a lista caçando o ícone âmbar incomodar na prática. A implementação certa é `WHERE NOT EXISTS (SELECT 1 FROM pdf_embeddings WHERE pdf_id = pdfs.id)` — SQL puro, compatível com a paginação por cursor, ~5 linhas, sem depender do estado derivado em Go. Isso encerra a "decisão aberta" do handoff anterior.
- **Sem rótulo textual de embedding nos cards.** Ideia minha, não pedido do usuário. O ícone âmbar com tooltip basta.
- **`MAX_UPLOAD_MB` fica em 200.** Baixar para 50 protegeria contra um caminho que o teto de 2 MB já corrigiu, ao custo de rejeitar upload legítimo.
- **`mem_limit` fica em `1g`.** Com 30 MiB medidos em regime normal, é folga de 33×, deliberada: o pico que importa é a extração de um PDF grande, e esse ainda não foi medido. Apertar para `256m` continua sendo 8× o regime — decisão adiada até haver medição do pico de extração.
- **A hipótese de OOM do incidente continua não confirmada.** O host reiniciou antes de qualquer acesso e o syslog do UNRAID vive em RAM. A evidência sumiu; as correções foram aplicadas sem ela, o que é o motivo de o `mem_limit` existir.

## 5. Pendência operacional, não de código

**Rotacionar duas credenciais**, expostas em texto puro durante a sessão (print de tela e comando de run colados no chat): `ADMIN_PASSWORD` e `GEMINI_API_KEY`. Gerar chave nova em `aistudio.google.com` e revogar a atual.

## 6. Ambiente — incantações que funcionam aqui

```bash
go build ./... && go vet ./... && go test ./...
cd frontend && npm run check && npm run build

# imagem real (o dist NÃO é versionado; o Docker constrói o frontend)
"C:/Program Files/Docker/Docker/resources/bin/docker.exe" build -t newpdfding:verify .
docker run -d --name npdverify --read-only --memory=1g -p 8907:8000 \
  -v npdverify-data:/data -v npdverify-files:/files \
  -e ADMIN_PASSWORD=... -e DB_PATH=/data/db.sqlite -e FILES=/files newpdfding:verify

# produção (somente leitura ao inspecionar!)
ssh -i C:/Users/EDalcin/Desktop/id_rsa_unraid root@192.168.1.10
sqlite3 -readonly /mnt/user/Storage/appsdata/newpdfding/db/newpdfding.db \
  'select count(*), min(length(embedding)/4) from pdf_embeddings;'
```

Armadilhas descobertas na prática:

- **`grep` de shell está bloqueado neste harness e trava o comando inteiro.** Usar `awk`.
- **O shell não tem rota de rede para `127.0.0.1` do Windows** (`curl` devolve `000`); containers auxiliares também não alcançaram o container em teste. Verificação por HTTP só pelo navegador.
- **A imagem é distroless:** `docker exec ... ls` falha, não há shell. Para inspecionar um volume, montá-lo em um container `alpine`.
- **Service worker antigo trava o app.** Reusar a porta de uma verificação anterior deixa a página presa em "Carregando…" para sempre, sem erro no console. Porta nova a cada rodada.
- **O navegador relay é o Chrome real do usuário** e o CDP dele degrada em sessões longas (`Runtime.callFunctionOn` estourando 20 s). Fazer as verificações de UI cedo, em chamadas curtas.
- Cookies de sessão são `Secure`; em `http://127.0.0.1` um reload perde a sessão.
- `newpdfding.db-wal` na casa de 10 MB é normal em WAL, não vazamento: o SQLite faz checkpoint sozinho e o arquivo desaparece quando o container para.

## 7. Documentação do projeto

- [`CONTEXT.md`](../CONTEXT.md) — glossário. Registra, entre outras coisas, a recusa deliberada do termo "pendente", que juntava `nenhum` e `desatualizado` num único nome e escondia que são situações diferentes.
- [`docs/adr/0001-modelo-de-embedding-fixo-no-codigo.md`](adr/0001-modelo-de-embedding-fixo-no-codigo.md)
- [`docs/adr/0002-sem-embedding-em-massa.md`](adr/0002-sem-embedding-em-massa.md)
- `refatoracao/` — especificação por etapa, atualizada junto com o código desta sessão.
