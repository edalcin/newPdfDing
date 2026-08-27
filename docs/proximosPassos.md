# Próximos passos

> Handoff da sessão de 2026-08-27 (segunda sessão do dia). Estado: `main` limpo, tudo empurrado.
> A sessão anterior deixou cinco tarefas pendentes. Duas foram feitas, duas foram **eliminadas por decisão** e uma virou gatilho, não tarefa.

---

## 1. Estado de produção

O banco em `192.168.1.10:/mnt/user/Storage/appsdata/newpdfding/db` foi zerado de vetores para começar do zero com `gemini-embedding-2`.

| | |
|---|---|
| Backup | `newpdfding.db.bak-20260827-164727` (`VACUUM INTO`, `integrity_check ok`, md5 `32e4341e1da290c84c30c64c39ee3216`) |
| Cópia fora do servidor | `D:/git/backups/newpdfding.db.bak-20260827-164727`, md5 idêntico |
| Antes | 167 PDFs, 117 vetores, 124 textos extraídos, 2 anotações, 57 tags |
| Depois | 167 PDFs, **0 vetores**, 124 textos extraídos, 2 anotações, 57 tags |
| Também apagado | linha `settings.ai_embed_model` (chave não existe mais no código) |
| Arquivo | 32,0 MB → 29,3 MB após `VACUUM` |
| Criado no host | `/mnt/user/Storage/appsdata/newpdfding/temp` (0777) |

`pdf_text` **não** foi tocado: texto extraído não depende de modelo, e apagá-lo forçaria 124 extrações de PDF desnecessárias — justamente o caminho que derrubou o host.

Todos os 167 documentos estão em estado `nenhum`. A busca léxica (FTS5) funciona sobre todos eles agora; a semântica só alcança o que for embedado.

## 2. O que mudou no código

**Modelo fixo.** `config.EmbedModel = "models/gemini-embedding-2"`, constante. Sumiram: a variável de ambiente `EMBED_MODEL`, o campo `cfg.EmbedModel`, a chave de configuração `ai.embed_model`, `Server.embedModelName()` e o pulldown em Configurações → IA. `/admin` exibe "Modelo de embedding (fixo): models/gemini-embedding-2", sem controle. `ai.text_model` (descrição e sugestão de tags) continua escolhível. Ver [ADR 0001](adr/0001-modelo-de-embedding-fixo-no-codigo.md).

**Sem embedding em massa.** Removidos `POST /api/admin/reembed`, `enqueueBulk`, `PendingEmbeddingIDs`, o botão "Reembedar pendentes" e o `pollInfo` de 5 s. Documentos são embedados um por vez pelo ícone de cérebro no card (`POST /api/pdfs/{id}/embed`, intocado). Ver [ADR 0002](adr/0002-sem-embedding-em-massa.md).

**Extração sai da RAM.** O temporário vai para `<FILES>/tmp` com `os.MkdirAll(..., 0o750)`, não mais para `/tmp` (que é RAM em UNRAID). `capText` limita o texto extraído a `extractedTextCapBytes = 2 MB` com `io.CopyN` + `buf.Grow`, espelhando `TEXT_LIMIT_BYTES` do navegador. `cleanOrphanedTempFiles` remove `npd-*.pdf` com mais de 6 h.

**`Stats()` para de carregar bodies inteiros.** Query única em streaming com `LEFT JOIN pdf_text` e `LEFT JOIN pdf_embeddings`, sem cláusula `IN` (que estouraria `SQLITE_MAX_VARIABLE_NUMBER` num acervo grande). O caminho paginado de 25 itens mantém o `IN` e ganhou `substr(body, 1, embedBodyChars)`. `TestAttachEmbeddingStatusHashSurvivesSQLCharTruncation` prova que o hash não mudou de valor — a invariante que, se quebrasse, marcaria o acervo todo como desatualizado.

**Limite de memória.** `mem_limit: 1g` no `compose.yaml`; `--memory=1g` em Extra Parameters no `UNRAID.md`. Um runaway futuro passa a ser restart de container, nunca queda de host.

## 3. Antes de subir a imagem no UNRAID

O template precisa de duas mudanças manuais na interface do Docker do UNRAID (o `UNRAID.md` já documenta as duas):

1. **Novo Path:** host `/mnt/user/Storage/appsdata/newpdfding/temp` → container `/files/tmp`, rw. Se esquecer, os temporários caem em `/files/tmp` dentro do volume `files` — ainda em disco, ainda seguro, só fora do diretório escolhido.
2. **Extra Parameters:** `--memory=1g`.

A variável `EMBED_MODEL`, se ainda estiver no template, pode ser removida: o binário a ignora.

## 4. Verificação feita, e o que ficou sem prova

Feito na imagem Docker real (`newpdfding:verify`, `--read-only --memory=1g`), pelo navegador em `http://127.0.0.1:8907`:

| Checado | Resultado |
|---|---|
| `GET /api/admin/info` | `"embed_model":"models/gemini-embedding-2"` presente, quatro contadores intactos |
| `POST /api/admin/reembed` | `405` — rota não existe |
| `PATCH /api/settings` com `ai.embed_model` | `400 unknown key "ai.embed_model"` |
| `/admin` | "Modelo de embedding (fixo): models/gemini-embedding-2", sem botão de lote |
| `/settings` → IA | só "Modelo para descrição e sugestão de tags"; pulldown de embedding sumiu |
| Container | `--memory=1g`, `--read-only`, healthy |

`go build`/`go vet`/`go test ./...` limpos; `npm run check` 0 erros/0 warnings; `npm run build` ok.

**Sem prova de container:** o caminho de extração (`extractPDFTextFromStorage`) não foi exercitado por HTTP dentro do container. Ele está coberto por teste comportamental — `TestExtractPDFTextFromStorage_TempFileUnderFilesTmp` intercepta `createTempFile` e afirma que o diretório é exatamente `<cfg.Files>/tmp` e nunca `os.TempDir()`, nos caminhos de sucesso **e** de erro, e que nada sobra depois —, mas não por um upload real seguido de clique em embedar. Motivo: o shell deste harness não tem rota de rede para o host (nem para os containers), e o CDP do navegador relay travou no meio da sessão.

**Como fechar essa lacuna em 30 segundos, no UNRAID:** embedar **um** documento pela interface e, com o container no ar, olhar se apareceu `/mnt/user/Storage/appsdata/newpdfding/temp` com nada dentro depois (o arquivo é apagado no fim da extração). Se aparecer e ficar vazio, o caminho está correto. Enquanto isso, `docker stats newpdfding` mostra o pico real de memória — que é também o número para refinar o `mem_limit`, hoje escolhido por raciocínio e não por medição.

## 5. Decisões desta sessão que não devem ser relitigadas

- **Sem filtro por estado de embedding na listagem.** Cogitado e recusado por não ser necessário: todos os 167 documentos estão em `nenhum`, então não há o que "achar", e `/admin` já responde quantos faltam. **Gatilho para implementar:** se rolar a lista caçando ícone âmbar incomodar na prática. A implementação certa é `WHERE NOT EXISTS (SELECT 1 FROM pdf_embeddings WHERE pdf_id = pdfs.id)` — SQL puro, compatível com o cursor, ~5 linhas, sem depender do estado derivado em Go. Isso encerra a "decisão aberta" da seção 3 do handoff anterior.
- **Sem rótulo textual de embedding nos cards.** Ideia minha da sessão passada, não pedido do usuário. O ícone âmbar com tooltip basta.
- **`MAX_UPLOAD_MB` fica em 200.** Baixar para 50 protegeria contra um caminho que o teto de 2 MB já corrigiu, ao custo de rejeitar upload legítimo.
- **A hipótese de OOM continua não confirmada.** O host reiniciou antes de eu chegar nele (up 2:52) e o syslog do UNRAID vive em RAM. A evidência sumiu; as correções foram aplicadas sem ela, o que é o motivo de o `mem_limit` existir.

## 6. Ambiente — incantações que funcionam aqui

```bash
go build ./... && go vet ./... && go test ./...
cd frontend && npm run check && npm run build

# imagem real (o dist NÃO é versionado; o Docker constrói o frontend)
"C:/Program Files/Docker/Docker/resources/bin/docker.exe" build -t newpdfding:verify .
docker run -d --name npdverify --read-only --memory=1g -p 8907:8000 \
  -v npdverify-data:/data -v npdverify-files:/files \
  -e ADMIN_PASSWORD=... -e DB_PATH=/data/db.sqlite -e FILES=/files newpdfding:verify

# produção
ssh -i C:/Users/EDalcin/Desktop/id_rsa_unraid root@192.168.1.10
sqlite3 -readonly /mnt/user/Storage/appsdata/newpdfding/db/newpdfding.db 'select count(*) from pdf_embeddings;'
```

Armadilhas descobertas na prática nesta sessão:

- **`grep` de shell está bloqueado neste harness e trava o comando inteiro.** Usar `awk`.
- **O shell não tem rota de rede para `127.0.0.1` do Windows** (`curl` devolve `000`), e containers auxiliares também não alcançaram o container em teste. Verificação por HTTP só pelo navegador.
- **A imagem é distroless:** `docker exec ... ls` falha, não há shell. Para inspecionar o volume, montar em um container `alpine`.
- **Service worker antigo trava o app.** Reusar a mesma porta de uma verificação anterior deixa a página presa em "Carregando…" para sempre, sem erro no console. Verificar em porta nova a cada rodada.
- Cookies de sessão são `Secure`; em `http://127.0.0.1` um reload perde a sessão.
