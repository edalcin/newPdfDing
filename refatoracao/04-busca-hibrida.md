# Busca híbrida

Este documento fixa o desenho da busca híbrida: índice léxico FTS5, embeddings semânticos Gemini sob demanda, e fusão por Reciprocal Rank Fusion (RRF) numa caixa única. É a implementação das decisões 4, 5 e 6 de [Visão geral](00-visao-geral.md) — caixa única de busca, embedding só sob acionamento manual (nunca automático), vetor como BLOB com cosseno calculado em Go.

As tabelas envolvidas (`pdfs_fts`, `pdf_text`, `pdf_embeddings`) estão definidas em [Modelo de dados](02-modelo-de-dados.md); o contrato HTTP completo (rotas, payloads, códigos de status) está em [API](05-api.md); o botão de embedding na interface está em [Frontend](06-frontend.md); as variáveis de ambiente `GEMINI_API_KEY` e `EMBED_MODEL` estão em [Docker, CI e deploy](07-docker-ci-deploy.md).

## Índice léxico (FTS5)

A tabela `pdfs_fts` é uma FTS5 virtual table **contentless** (`content=''`): não guarda cópia dos dados, só o índice invertido. Colunas indexadas: `name`, `description`, `notes`, `body` (texto extraído do PDF, de `pdf_text`) e `tags` (nomes de tag do PDF concatenados), com `tokenize='unicode61 remove_diacritics 2'`.

### Mapeamento de rowid

Toda tabela FTS5 é indexada por um `rowid` inteiro. Mas `pdfs.id` é `TEXT` (UUIDv7), não `INTEGER`. A solução fixada: **não declarar `pdfs` como `WITHOUT ROWID`**, deixando o SQLite manter a coluna `rowid` implícita de qualquer tabela comum, e usar essa coluna como ponte entre as duas tabelas:

```sql
SELECT p.id
FROM pdfs_fts
JOIN pdfs p ON p.rowid = pdfs_fts.rowid
WHERE pdfs_fts MATCH ?
ORDER BY pdfs_fts.rank
LIMIT 100;
```

Se `pdfs` for declarada `WITHOUT ROWID` — uma otimização comum para tabelas com chave primária `TEXT` — essa coluna some e o join acima quebra. **`pdfs` nunca pode ser `WITHOUT ROWID`.**

### Reindexação

Toda escrita que muda conteúdo pesquisável — criação/edição de um PDF (`name`/`description`/`notes`), mudança nas tags de um PDF, ou gravação/atualização do texto extraído em `pdf_text` — reindexa `pdfs_fts` **dentro da mesma transação** da escrita de origem. Não há reindexação assíncrona nem em lote.

Por ser contentless, `pdfs_fts` não guarda os valores antigos: o comando de remoção do FTS5 (`'delete'`) exige a linha inteira anterior, não só o `rowid`:

```sql
-- remove a entrada antiga (valores lidos antes do UPDATE/DELETE)
-- ordem dos parâmetros: rowid, name, description, notes, body, tags (valores antigos)
INSERT INTO pdfs_fts(pdfs_fts, rowid, name, description, notes, body, tags)
VALUES ('delete', ?, ?, ?, ?, ?, ?);

-- (re)insere a entrada atual — omitido quando a operação é uma exclusão de PDF
-- ordem: rowid, name, description, notes, body, tags (valores atuais)
INSERT INTO pdfs_fts(rowid, name, description, notes, body, tags)
VALUES (?, ?, ?, ?, ?, ?);
```

Como `pdfs_fts` é uma virtual table, o `ON DELETE CASCADE` do SQL não a alcança: ao excluir um PDF, o código de aplicação precisa emitir o `'delete'` acima (com o `rowid` e os valores antigos, lidos antes do `DELETE FROM pdfs`) dentro da mesma transação da exclusão.

### Rebuild total no boot

No boot, o índice inteiro é reconstruído a partir do estado atual das tabelas de origem — corrige qualquer divergência acumulada sem depender de tracking incremental:

```sql
INSERT INTO pdfs_fts(pdfs_fts) VALUES ('delete-all');

INSERT INTO pdfs_fts(rowid, name, description, notes, body, tags)
SELECT p.rowid, p.name, p.description, p.notes,
       COALESCE(pt.body, ''),
       COALESCE(tl.tags, '')
FROM pdfs p
LEFT JOIN pdf_text pt ON pt.pdf_id = p.id
LEFT JOIN (
  SELECT pdf_tags.pdf_id, group_concat(tags.name, ' ') AS tags
  FROM pdf_tags JOIN tags ON tags.id = pdf_tags.tag_id
  GROUP BY pdf_tags.pdf_id
) tl ON tl.pdf_id = p.id;
```

## Consulta léxica

A busca tenta primeiro FTS5. Para evitar que caracteres da sintaxe de query do FTS5 (`AND`, `OR`, `-`, `:`, etc.) quebrem uma busca livre digitada pelo usuário, a string é **envolvida em aspas duplas**, com qualquer aspa dupla literal escapada dobrando-a (`"` → `""`), fazendo o FTS5 tratar a entrada inteira como frase literal:

```go
matchQuery := `"` + strings.ReplaceAll(userQuery, `"`, `""`) + `"`
```

Resultado ordenado por `pdfs_fts.rank` (melhor primeiro). **Se a consulta FTS5 devolver zero resultados ou erro de sintaxe**, a busca cai para `LIKE`:

```sql
SELECT id FROM pdfs
WHERE name LIKE '%' || ? || '%' OR description LIKE '%' || ? || '%'
LIMIT 100;
```

Esse fallback substitui o RapidFuzz (fuzzy matching em Python) do produto atual.

## Backfill de texto (`pdf_text` ausente)

`pdf_text` só é preenchido no momento em que o texto é conhecido: upload pelo navegador (pdf.js extrai e envia junto no `multipart`) ou watch-dir (extração pura-Go, ver [Arquitetura](01-arquitetura.md)). Um PDF pode chegar **sem** essa linha por dois caminhos: a importação do banco Django legado (`ETAPA-12-IMPORTACAO`, que não tenta OCR/parsing) e um upload cujo pdf.js falhou no navegador (degradação graciosa, ver [Frontend](06-frontend.md)).

Para esses casos, `POST /api/pdfs/{id}/text` (contrato em [API](05-api.md)) faz o *upsert* de `pdf_text` e reindexa `pdfs_fts` na mesma transação — mesma mecânica de reindexação descrita acima, só que disparada fora do fluxo de criação/edição do PDF. O visualizador chama essa rota automaticamente na primeira abertura de um documento cujo `has_text` (campo derivado em `GET /api/pdfs/{id}`) é `false`, extraindo o texto no navegador com o mesmo pdf.js do upload. Enquanto isso não acontece, o documento continua pesquisável por nome/descrição/tags (a busca nunca falha por falta de corpo de texto) e o botão de embedding devolve `422` (ver [Embedding sob demanda](#embeddings-sob-demanda)) até o backfill ocorrer.

## Embeddings sob demanda

Texto embutido para cada PDF, fórmula fixada:

```go
const embedBodyChars = 2000

text := pdf.Name + "\n" + pdf.Description + "\n" + firstNChars(pdfText.Body, embedBodyChars)
```

`embedBodyChars = 2000` é maior que o equivalente em `pkd` porque aqui o corpo é o texto extraído de um PDF inteiro, não uma nota curta.

Hash de conteúdo, gravado junto com o vetor em `pdf_embeddings.content_hash`:

```
content_hash = sha256(EMBED_MODEL + "\x00" + text)
```

Cada acionamento do botão dispara **uma chamada à API com um único texto** — não existe endpoint de embedding em lote, porque não existe processamento em massa (decisão 5 em [Visão geral](00-visao-geral.md)).

## Chamada à API Gemini

Formato literal da chamada (autenticação por header, não por query string — ver "Autenticação por header" abaixo):

```http
POST https://generativelanguage.googleapis.com/v1beta/{EMBED_MODEL}:batchEmbedContents
Content-Type: application/json
x-goog-api-key: {GEMINI_API_KEY}
```

Corpo (literal): `{"requests":[{"model":"<EMBED_MODEL>","content":{"parts":[{"text":"..."}]}}]}`

Resposta (literal): `{"embeddings":[{"values":[...]}]}`

Expandido para leitura:

```json
{
  "requests": [
    {
      "model": "<EMBED_MODEL>",
      "content": { "parts": [{ "text": "..." }] }
    }
  ]
}
```

```json
{
  "embeddings": [
    { "values": [/* floats do vetor de embedding */] }
  ]
}
```

Default: `EMBED_MODEL=models/gemini-embedding-001` — variável documentada em [Docker, CI e deploy](07-docker-ci-deploy.md). A seleção em Configurações → IA (`settings['ai.embed_model']`, ver [Modelo de dados](02-modelo-de-dados.md)), quando preenchida, tem precedência sobre a variável.

Referência de implementação Go (mesma forma de `pkd` `internal/store/semantic.go:embedBatch`, adaptada para um texto por chamada):

```go
req := reqBody{
    Requests: []embReq{
        {
            Model: model,
            Content: embContent{
                Parts: []embPart{{Text: text}},
            },
        },
    },
}
// POST com Content-Type: application/json
// decodifica out.Embeddings[0].Values
```

## Autenticação por header

Todas as chamadas ao Gemini (embed, listagem de modelos, geração de texto) enviam a chave no header `x-goog-api-key`, nunca na query string. Motivo: quando `HTTPClient.Do` falha, o erro devolvido é `*url.Error`, cujo `Error()` inclui a URL completa — e o `log.Printf` de erro do handler gravaria a `GEMINI_API_KEY` no log do servidor se ela estivesse na URL. Documentado pela própria API Gemini em https://ai.google.dev/gemini-api/docs/api-key.

## Modelos de IA — listagem e geração de texto

Além de `Embed`, `GeminiClient` expõe dois métodos usados pela área "Configurações → IA" e pelos botões "Descrever com IA"/"Sugerir tags" na página do PDF (contrato HTTP completo em [API](05-api.md)):

- **`ListModels(ctx) (embed, text []GeminiModel, err error)`** — `GET /v1beta/models?pageSize=1000`, paginado (máximo 5 páginas), dividindo o catálogo por capacidade: `embed` = modelos com `embedContent` em `supportedGenerationMethods`; `text` = modelos com `generateContent`, exceto os que caem numa deny list de substrings do nome (`-tts`, `-image`, `imagen`, `veo`, `aqa`, `embedding`) — modelos que não devolvem prosa plana.
- **`GenerateText(ctx, model, system, prompt) (string, error)`** — `POST /v1beta/{model}:generateContent` com uma instrução de sistema e um prompt de usuário, `generationConfig: {temperature: 0.2, maxOutputTokens: 2048}` (margem para modelos com "thinking" ligado por padrão, que consomem orçamento de saída antes de escrever). Devolve o texto concatenado do primeiro candidato; resposta vazia vira erro com o `finishReason` embutido.

O modelo de embedding é resolvido a cada chamada via `Server.embedModelName()` — `settings['ai.embed_model']` quando preenchido, senão `EMBED_MODEL` do ambiente — nunca fixado uma única vez na inicialização do servidor, porque a seleção em Configurações pode mudar em runtime. O modelo de texto (`settings['ai.text_model']`) não tem default: os dois botões respondem `412` até o usuário escolher um.

## Armazenamento vetorial

O vetor é gravado em `pdf_embeddings.embedding` como `float32` little-endian concatenados num `BLOB`, via um par de funções `encodeEmbedding`/`decodeEmbedding`. A normalização L2 acontece **na gravação**, não na consulta — assim a similaridade de cosseno na busca é um produto escalar puro, sem recalcular norma a cada comparação.

`gemini-embedding-001` produz vetores de **3072 dimensões**: 3072 × 4 bytes ≈ 12 KB por vetor; a 20.000 PDFs isso é ~245 MB de BLOB acumulado em `pdf_embeddings`. **Contingência pré-decidida, não implementada nesta refatoração**: se esse tamanho (ou o custo do KNN, abaixo) incomodar, pedir `outputDimensionality: 768` na chamada `batchEmbedContents` — o modelo suporta truncamento MRL (Matryoshka Representation Learning) — e renormalizar o vetor truncado, sem trocar de modelo nem de arquitetura.

## Sem worker, sem automatismo

Decisão explícita, não um detalhe de implementação em aberto: **não existe** goroutine de varredura, `time.Ticker`, canal `notify()`, nem varredura no boot para embedding. `EMBED_SWEEP_MINUTES` **não existe** como variável de ambiente — o único processo periódico do produto é o consumo por watch-dir (`CONSUME_INTERVAL_MINUTES`); embedding não é ele, e não há job de backup nesta refatoração (decisão 8 em [Visão geral](00-visao-geral.md)).

O **único** caminho de código que grava em `pdf_embeddings` é o handler de `POST /api/pdfs/{id}/embed` (contrato completo em [API](05-api.md)). Um `sync.Mutex` no servidor serializa os acionamentos do botão: nunca há duas chamadas simultâneas à API Gemini. Um segundo clique enquanto o primeiro ainda está em curso recebe `409`.

## Estado de embedding (`embedding_status`)

Campo derivado devolvido em `GET /api/pdfs` e `GET /api/pdfs/{id}` (contrato em [API](05-api.md)), com exatamente três valores possíveis:

| Estado | Condição | Botão | Rótulo |
|---|---|---|---|
| `none` | não existe linha em `pdf_embeddings` para o `pdf_id` | habilitado | "Embedar" |
| `current` | existe linha e `content_hash` bate com o hash do conteúdo atual | desabilitado | "Embedado" |
| `stale` | existe linha mas `content_hash` diverge (nome, descrição ou texto mudaram, ou `EMBED_MODEL` mudou) | habilitado | "Reembedar" |

O hash atual **não** é uma coluna extra no schema — é recalculado em Go a cada leitura, sobre os mesmos campos usados na gravação:

```go
current := sha256Hex(embedModel + "\x00" + buildEmbedText(pdf, pdfText))

switch {
case row == nil:
    status = "none"
case row.ContentHash == current:
    status = "current"
default:
    status = "stale"
}
```

Sem o estado `stale`, um documento editado depois de embedado ficaria com um vetor desatualizado para sempre, sem nenhum caminho de correção pela interface — daí o terceiro estado, mesmo custando um hash recalculado a cada leitura da listagem.

## Remoção do vetor

`DELETE /api/pdfs/{id}` remove a linha de `pdf_embeddings` por `ON DELETE CASCADE` (FK declarada no schema). Arquivar ou desarquivar um PDF **não** apaga o vetor — o documento apenas sai (ou volta a entrar) da lista de candidatos da busca enquanto estiver arquivado, pela mesma cláusula de filtro `archived` usada nos demais filtros (ver [Filtros combinados com a busca](#filtros-combinados-com-a-busca)).

## Fusão RRF

Toda busca roda os dois candidatos (léxico via FTS5/LIKE, semântico via cosseno) e funde por Reciprocal Rank Fusion, `k = 60`:

```go
const rrfK = 60.0

// lexical e semantic são listas ordenadas de pdf IDs (melhor primeiro)
func fuseRRF(lexical, semantic []string, limit int) []string {
    score := map[string]float64{}
    for rank, id := range lexical {
        score[id] += 1.0 / (rrfK + float64(rank+1))
    }
    for rank, id := range semantic {
        score[id] += 1.0 / (rrfK + float64(rank+1))
    }
    // ordena por score desc, desempate por ID asc (determinismo)
    ...
}
```

Cada lista de entrada tem no máximo 100 candidatos; o resultado da fusão é cortado em `limit` (default 50).

Documentos sem embedding simplesmente não aparecem na lista `semantic` — isso não é um erro nem um caso especial tratado em código. Sem `GEMINI_API_KEY` configurada, ou enquanto nenhum documento estiver embedado, `semantic` é sempre a lista vazia, e o RRF degrada exatamente para a ordem do FTS5/LIKE — **sem nenhum `if` extra** distinguindo "busca híbrida" de "busca só léxica" no código.

## Piso semântico e top-k

Candidatos semânticos com cosseno `< 0.30` são descartados **antes** de entrar na fusão. Top-k semântico = 100.

> **Nota comparativa**: `pkd` usa `semanticQueryFloor = 0.45` e `semanticQueryTopK = 50` para busca por query. Aqui o piso é mais baixo (0.30) e o top-k maior (100) porque quem decide o peso final de cada resultado é a fusão RRF por posição de rank, não um corte rígido de similaridade — um candidato semântico fraco pode entrar na lista sem dominar o resultado, já que sua posição de rank ruim já o penaliza na fórmula RRF.

## Custo

O cálculo de cosseno para a busca semântica é uma varredura completa em memória, `O(n·d)` — sem índice, sem ANN (approximate nearest neighbor). Teto documentado: aceitável até **~20.000 PDFs**. Acima disso, o caminho de upgrade é substituir por `sqlite-vec` — o que exigiria CGO, contrariando as decisões 1 e 6 ([Visão geral](00-visao-geral.md)). Este é um caminho de upgrade **registrado, não implementado** nesta refatoração.

## Filtros combinados com a busca

Filtros de `tag`, `collection`, `starred` e `archived` aplicam-se como `WHERE` **depois** da fusão RRF (ou depois da ordenação simples por FTS5/LIKE, quando não há termo de busca), sobre a lista de IDs já ordenada:

```sql
SELECT * FROM pdfs
WHERE id IN (/* IDs na ordem produzida pela fusão */)
  AND (:tag = '' OR id IN (SELECT pdf_id FROM pdf_tags WHERE tag_id = :tag))
  AND (:collection = '' OR collection_id = :collection)
  AND (:starred_only = 0 OR starred = 1)
  AND archived = :archived;
```

A ordem final devolvida ao cliente preserva a ordem de rank da fusão (ou da ordenação FTS5/LIKE), não a ordem natural do `SELECT`.
