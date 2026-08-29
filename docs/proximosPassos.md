# Próximos passos

> Handoff de 2026-08-29. Estado: `main` em `2a84400` + as mudanças desta sessão, CI verde, produção em operação no UNRAID.
> A sessão anterior fechou todas as pendências de código. Esta abriu e fechou três: um defeito de dados que travava o embedding, ícones que só atualizavam com recarga de página, e o filtro por estado de embedding — este último **revertendo uma decisão** registrada no handoff anterior.

---

## 1. Defeito corrigido: "Reembedar" eterno (commit `2a84400`)

**Sintoma relatado:** um PDF (`019fa385-2765-7f18-8ac3-fe95e263fa5c`, *Politica de Dados JBRJ 2021*) voltava para "Reembedar" logo após terminar de embedar, indefinidamente.

**Causa-raiz:** duas unidades de truncagem para o mesmo hash.

| caminho | corte | resultado neste PDF |
|---|---|---|
| escrita (`runEmbedJob` → `buildEmbedText`) | 2000 **bytes**, em Go | 2000 bytes → hash `347dfe79…` gravado |
| leitura (`attachEmbeddingStatus`, `Stats`) | `substr(body, 1, 2000)` = 2000 **caracteres** | 936 bytes → hash `9fd08878…` |

As funções de texto do SQLite (`length`, `substr`) **param no primeiro NUL**. O texto extraído deste PDF tem NUL no byte 937 de 14165 (`length(body)` = 920, `length(CAST(body AS BLOB))` = 14165 — a discrepância é o diagnóstico). O hash recomputado a cada leitura nunca podia bater com o gravado. O embedding sempre funcionou; a verificação estava errada.

**Correção:** `substr(CAST(body AS BLOB), 1, ?)` nos dois caminhos de leitura — conta bytes, casa exatamente com `buildEmbedText`.

**Sem reembedar nada.** Para corpo UTF-8 válido o prefixo de 2000 bytes é idêntico ao de antes, então nenhum hash já gravado muda. Recomputado com os dados reais de produção, o hash deste PDF dá exatamente o valor gravado (`347dfe7995843ba3…`) → volta a `current` sozinho quando a imagem subir. No acervo: 125 PDFs com texto, **5** com NUL nos primeiros 2000 bytes, **1** já embedado e afetado (justamente o relatado).

**Correção de registro:** o handoff anterior afirmava que `TestAttachEmbeddingStatusHashSurvivesSQLCharTruncation` provava que o hash não mudava de valor. Provava só para UTF-8 válido — a premissa "todo caractere tem ≥ 1 byte, logo 2000 caracteres contêm ≥ 2000 bytes" cai quando existe NUL. O teste agora inclui um corpo com NUL e **falha** no código antigo.

## 2. Ícones de embedding ao vivo, em qualquer página

**Sintoma relatado:** era preciso recarregar a página para ver que o embedding terminou.

**Causa:** o polling só existia *depois* de um clique e *dentro* do componente que foi clicado (`embedJobs.watch(pdfId, callback)` chamado no `handleClick`). Recarregar, navegar ou abrir a biblioteca com um job já em andamento no servidor deixava zero polling ativo.

**Inversão:** quem exibe ícone se inscreve, e a inscrição liga o polling.

- `embedJobs.onSettled(listener)` → devolve a função de cancelamento; a biblioteca (`routes/+page.svelte`) e a página de detalhes (`routes/pdf/[id]/+page.svelte`) se inscrevem via `$effect`. Ao sair do mapa de jobs, o `pdf_id` é emitido e a página refaz o `GET /api/pdfs/{id}` do documento afetado.
- O botão (`embed-button.svelte`) perdeu a prop `onUpdated` e o `watch`: só enfileira e chama `embedJobs.poll()` uma vez, para o rótulo sair de "Embedar" na hora. `pdf-card.svelte` perdeu a prop `onEmbedUpdated`.
- Duas cadências: **1,5 s** enquanto há job em estado não terminal (`queued`/`extracting`/`embedding`), **10 s** de batimento caso contrário. Um job `failed` fica no mapa do servidor até ser reenfileirado e um `done` fica 60 s — nenhum dos dois justifica 1,5 s indefinidamente.
- Aba oculta não gera requisição; ao voltar a ficar visível, poleia na hora.
- Página sem ícone de embedding não tem inscrito e portanto **não poleia**.

## 3. Filtro por estado de embedding na listagem

Pedido explícito do usuário — **reverte** a decisão "sem filtro por estado de embedding" do handoff anterior (que previa o gatilho "se rolar a lista caçando o ícone âmbar incomodar na prática"; incomodou).

`GET /api/pdfs?embedding=none|current|stale`, validado no handler; valor inválido → `400 invalid embedding`.

**Por que não foi o `WHERE NOT EXISTS` que o handoff anterior prescrevia:** aquele SQL só resolve `none`. `current` e `stale` se distinguem **apenas** pela comparação do `content_hash` gravado com o hash recomputado do conteúdo atual — sha256 que o SQLite não calcula. Não existe coluna para filtrar.

**Implementação:** `List` ganhou o ramo `listFilteredByEmbedding`, que varre a mesma sequência ordenada que `listPage` pagina, em lotes de 200, mantendo só as linhas cujo status derivado casa, e devolve como cursor o cursor da **última linha mantida** — a rolagem infinita continua com o mesmo contrato de cursor opaco. Na busca (`q!=""`) o filtro é aplicado depois do status derivado, sobre a página única já fundida por RRF.

Custo: no pior caso uma varredura completa de `pdfs` + prefixo de `pdf_text`; com 167 documentos é um lote só.

**UI:** três chips na biblioteca ("Sem embedding", "Atualizado", "Desatualizado"), com os mesmos ícones da legenda. Clicar no chip ativo desliga o filtro.

## 4. Verificação desta sessão

Backend:

| Checado | Resultado |
|---|---|
| `TestAttachEmbeddingStatusHashSurvivesSQLCharTruncation` (agora com corpo NUL) | falha no código antigo (`status = "stale"`), passa no novo |
| `TestListFilterByEmbeddingStatus` | os três estados, uma página por vez (`Limit=1`), cursor sem repetir nem pular item |
| Hash recomputado com os dados reais de produção | idêntico ao gravado |
| `go build ./...`, `go test ./internal/...` | limpos |
| `npm run check` | 0 erros / 0 warnings |

Frontend, dirigindo o Chrome real contra uma instância local (`localhost:8099`, banco próprio):

| Cenário | Observado |
|---|---|
| Página carregada com job **já em andamento**, sem nenhum clique | "Na fila…" → "Extraindo texto…" → "Embedando…", acompanhando as fases sozinho |
| Job sai do mapa | **1** refetch automático, rótulo "Embedado", ícone `bxs-brain text-primary`, ponto vermelho removido, **sem recarregar** |
| Biblioteca ociosa, 24 s | 2 requisições a `/api/embed/jobs` (batimento de 10 s) |
| Página `/tags`, 24 s | 0 requisições |
| Filtro `none` / `current` / `stale` | exatamente os documentos semeados em cada estado; desligar restaura os quatro |

Só o snapshot `/api/embed/jobs` foi faturado (fica no lugar do worker, que exigiria chamada real ao Gemini). Refetch, status derivado e filtro vieram do servidor de verdade.

## 5. O que fica em aberto

- **Nada de código.** Uma suspeita levantada e **descartada por medição**: se o índice FTS5 dos 5 PDFs com NUL também paravam no primeiro NUL. Não param. No PDF `019fa385` a palavra "restritos", que fica no byte ~3000 (o NUL está no 937), casa em `pdfs_fts MATCH` para exatamente esse documento. O tokenizador `unicode61` trata o NUL como separador; só as funções escalares (`length`, `substr`) sobre TEXT param nele — que é o defeito da seção 1, já corrigido. O texto extraído continua com NUL, o que é feio no que vai para a API, mas não quebra busca léxica, embedding nem status. **Sanitizar NUL na extração fica recusado:** mudaria o hash desses documentos e forçaria reembedar sem consertar nada observável.
- **`extractPDFTextFromStorage` continua sem prova de produção** (mesma pendência do handoff anterior; ver o `watch` da seção 7).
- **Rotacionar `ADMIN_PASSWORD` e `GEMINI_API_KEY`**, expostas em texto puro em sessão anterior. Pendência operacional, não de código.

## 6. Decisões que não devem ser relitigadas

- **Truncagem por bytes, não sanitização de NUL** (seção 1). Sanitizar invalidaria os vetores dos documentos com NUL e forçaria reembedar, e a medição da seção 5 mostra que não haveria ganho observável: a busca léxica já alcança o texto depois do NUL. A correção escolhida conserta a verificação com custo zero de reembedding.
- **Filtro de embedding é derivado em Go, não SQL** (seção 3). Só `none` caberia em SQL puro; ter dois mecanismos para o mesmo filtro seria pior que um lote de 200 linhas.
- **Sem rótulo textual de embedding nos cards.** O ícone com tooltip basta.
- **`MAX_UPLOAD_MB` fica em 200.** `mem_limit` fica em `1g` até haver medição do pico de extração.
- **A hipótese de OOM do incidente continua não confirmada** — o syslog do UNRAID vive em RAM e o host reiniciou antes de qualquer acesso.

## 7. Ambiente — incantações que funcionam aqui

```bash
go build ./... && go vet ./... && go test ./...
cd frontend && npm run check && npm run build

# rodar local: o dist NÃO é versionado, e sem esta copia TODO asset volta
# como index.html (2045 B) e a SPA morre com erro de MIME no console
cp -r frontend/build/. internal/server/web/dist/
go build -o /tmp/npd/newpdfding.exe ./cmd/newpdfding
DB_PATH=... ADMIN_PASSWORD=... FILES=... LISTEN_ADDR=:8099 /tmp/npd/newpdfding.exe
# ao terminar: git checkout -- internal/server/web/dist/index.html

# produção (somente leitura ao inspecionar!)
ssh -i ~/.ssh/id_rsa_unraid root@192.168.1.10   # o caminho do Desktop nao e alcancavel pelo shell
sqlite3 'file:/mnt/user/Storage/appsdata/newpdfding/db/newpdfding.db?mode=ro' \
  'select count(*), min(length(embedding)/4) from pdf_embeddings;'
```

Armadilhas descobertas na prática:

- **`length(body)` em SQLite conta caracteres e para no primeiro NUL; `length(CAST(body AS BLOB))` conta bytes.** Divergência entre os dois é a assinatura de texto extraído com NUL. Vale para `substr` também.
- **`curl` não autentica na instância local por HTTP:** o cookie de sessão é `Secure` e o `curl` se recusa a enviá-lo em `http://` → `403 missing CSRF cookie`. O Chrome trata `localhost` como origem confiável e envia. Verificação de UI só pelo navegador.
- **`grep` de shell está bloqueado neste harness e trava o comando inteiro.** Usar a ferramenta `grep` do harness ou `awk`.
- **Service worker antigo trava o app.** Reusar a porta de uma verificação anterior deixa a página presa em "Carregando…" sem erro no console. Porta nova a cada rodada, ou desregistrar o SW antes do `goto`.
- **A imagem é distroless:** não há shell para `docker exec`. Para inspecionar um volume, montá-lo em um `alpine`.
- **O `sqlite3` existe no host UNRAID** em `/usr/bin/sqlite3`; não é preciso container auxiliar para inspecionar o banco.
- `newpdfding.db-wal` na casa de 10 MB é normal em WAL, não vazamento.

## 8. Documentação do projeto

- [`CONTEXT.md`](../CONTEXT.md) — glossário. Registra a recusa deliberada do termo "pendente", que juntava `nenhum` e `desatualizado` num único nome.
- [`docs/adr/0001-modelo-de-embedding-fixo-no-codigo.md`](adr/0001-modelo-de-embedding-fixo-no-codigo.md)
- [`docs/adr/0002-sem-embedding-em-massa.md`](adr/0002-sem-embedding-em-massa.md)
- `refatoracao/` — especificação por etapa.
