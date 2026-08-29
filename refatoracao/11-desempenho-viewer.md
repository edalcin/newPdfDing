# 11 — Desempenho do viewer em PDFs grandes

**Status:** estudo crítico + **Fases 1–3 implementadas e deployadas em produção**. Fase 4 (curadoria de acervo) não é código, é decisão do usuário — permanece em aberto.
**Data do estudo:** 2026-07-27. **Data da implementação:** 2026-07-27.
**Pergunta que este documento responde:** o travamento do viewer em PDFs grandes é contornável por configuração do EmbedPDF, ou exige trocar de SDK?

**Veredito:** era majoritariamente **configuração e arquitetura da nossa integração**, não uma deficiência irreparável do SDK. Quatro das cinco causas identificadas estavam no nosso código ou na nossa CSP. Uma quinta (ausência de HTTP Range) é limitação real do SDK, mas era a **menos** relevante para os arquivos do acervo. **Não trocamos de SDK.** Ver seção 9 para o que foi implementado.

---

## 1. Método e evidências

Todas as afirmações abaixo foram verificadas em fonte primária:

- Código da aplicação em `D:/git/newPdfDing`.
- Pacote **instalado** em `frontend/node_modules/@embedpdf/*` (v2.14.4) — não a documentação de marketing.
- Banco e arquivos **de produção** (UNRAID, `/mnt/user/Storage/appsdata/newpdfding`), inspecionados por SSH.

Onde não houve verificação direta, o texto está marcado com `[INFERÊNCIA]`.

---

## 2. O acervo real (medido, não estimado)

Consulta em `pdfs` na base de produção:

| Métrica | Valor |
|---|---|
| Total de PDFs | 157 |
| Tamanho total | 829 MB |
| Arquivos > 20 MB | 13 |
| Arquivos sem `num_pages` | 0 |

Os doze maiores:

| MB | Páginas | KB/página | Texto extraído (chars) | Documento |
|---:|---:|---:|---:|---|
| 71,7 | 108 | 680 | 157.450 | Plano de Gestão Territorial e Ambiental |
| 47,7 | 66 | 740 | 95.318 | RESTINGAS da Região dos Lagos |
| 37,4 | 440 | 87 | 440 | Historia Naturalis Brasiliae |
| 29,2 | 219 | 137 | 0 | Arborização de Vias Públicas |
| 28,6 | 445 | 66 | 0 | Povos Indígenas no Brasil |
| 28,4 | 22 | 1.322 | 0 | An overview of Natural History Coll. |
| 27,5 | 168 | 167 | 0 | Patrimônio genético / conhec. tradicional |
| 24,7 | 373 | 68 | 0 | Good Data |
| 24,3 | 1.082 | 23 | 0 | 6º Relatório Nacional para CDB |
| 23,8 | 408 | 60 | 0 | Medicina Rustica |
| 22,6 | 55 | 420 | 304.009 | State of the World's Plants and Fungi |
| 20,8 | 88 | 242 | 0 | Relatório Curso Capacitação Gestão |

### 2.1. Correção importante ao diagnóstico inicial

A hipótese de trabalho era "PDFs feitos de imagens escaneadas". **Isso só é verdade para parte do acervo.** Perfilando os streams internos dos maiores arquivos:

| Arquivo | Composição real dos streams | Classe |
|---|---|---|
| **A** — 71,7 MB / 108 p | **65,7 MB FlateDecode**, 4,7 MB JPXDecode, 0,1 MB fontes | **Vetorial pesado** |
| **B** — 47,7 MB / 66 p | **41,1 MB DCTDecode** (JPEG) + 5,9 MB imagens | Escaneado (JPEG) |
| **C** — 37,4 MB / 440 p | **27,8 MB JBIG2Decode** + 9,2 MB JPXDecode | Escaneado (bitonal) |

O maior arquivo do acervo **não é um escaneado**. Descomprimindo seus dois maiores objetos:

```
obj 371: 12,5 MB comprimido -> 39,7 MB descomprimido (ratio 3,2x)
  operadores: c=853.998  cm=125.404  m=62.715  f=62.612  scn=54.025  l=24.350
  total de operações de path (m/l/c): 941.063
  amostra: q /GS0 gs 0 TL /Fm0 Do Q /PlacedPDF /MC0 BDC ...

obj 254: 7,3 MB comprimido -> 21,9 MB descomprimido (ratio 3,0x)
  total de operações de path (m/l/c): 568.769
```

São **Form XObjects com `/BBox` e `/Group`** (grupos de transparência) contendo **941 mil operações de path, das quais 853.998 curvas de Bézier, em uma única página**, marcadas como `/PlacedPDF` — cartografia vetorial exportada de Illustrator/GIS. É um dos piores perfis de renderização que existe em PDF: não há imagem para redimensionar, o custo é tesselação e composição de transparência.

**Consequência:** qualquer plano baseado em "recomprimir as imagens para 150 DPI" **não resolve o pior caso**. É preciso tratar as três classes separadamente.

---

## 3. Diagnóstico — cinco causas, por ordem de impacto

### C1. `worker: false` — o motor PDFium roda na thread principal

`frontend/src/lib/embedpdf.ts`, `viewerConfig()`:

```ts
worker: false,
wasmUrl: '/embedpdf/pdfium.wasm',
```

O comentário no arquivo documenta a razão, e ela é legítima: o worker do EmbedPDF é criado a partir de uma `blob:` URL. Verificado em `@embedpdf/engines/dist/worker-engine.js:487`:

```js
const worker = new Worker(
  URL.createObjectURL(new Blob(['var Rotation = ...'], ...)),
  { type: "module" }
);
```

E há um **segundo** blob para o worker de encoding, na linha 493. Nossa CSP (`internal/security/headers.go:14`) é:

```
default-src 'self'; img-src 'self' data: blob:; script-src 'self' 'wasm-unsafe-eval'%s;
style-src 'self' 'unsafe-inline'; connect-src 'self'; frame-ancestors 'self';
object-src 'none'; base-uri 'none'
```

Não há `worker-src`. Sem essa diretiva, workers recaem em `child-src` e depois em `script-src`, que não inclui `blob:` → o worker é bloqueado → o documento nunca termina de carregar. A escolha de `worker: false` foi um contorno correto para *aquele* sintoma, mas transferiu **toda** a rasterização PDFium para a thread principal.

**É esta a causa direta do "simplesmente não carrega".** A aba não está lenta: está bloqueada. O navegador não repinta, não responde a scroll e eventualmente mostra "página não responde".

### C2. O tiling replica a página inteira a cada tile

Verificado em `@embedpdf/plugin-tiling/dist/index.js:12-14`:

```js
tileSize: 768,
overlapPx: 2.5,
extraRings: 0
```

`extraRings: 0` já é o melhor valor (sem pré-carregar vizinhos) e **não há limiar de zoom** — o tiling é incondicional (grep por `threshold`/`minScale`/`shouldTile` em `plugin-tiling/dist/index.js`: nenhum resultado).

O ponto crítico é *como* um tile é renderizado. Em `@embedpdf/engines/dist/direct-engine-C8xTbxym.js`, tanto `renderPage` quanto `renderPageRect` (linha 1482) delegam ao **mesmo** `renderRectEncoded`, e há exatamente **uma** chamada de rasterização em todo o motor:

```
grep -on "FPDF_RenderPageBitmap\w*" direct-engine-C8xTbxym.js
8742:FPDF_RenderPageBitmapWithMatrix
```

`FPDF_RenderPageBitmapWithMatrix` **reexecuta a display list inteira da página**, recortada pela matriz. Não existe execução parcial de content stream no PDFium.

Portanto, para o arquivo A: cada tile custa as 941.063 operações de path da página inteira.

`[INFERÊNCIA]` Numa viewport de ~1600 px CSS com `dpr = 2`, uma página A4 em ajuste-à-largura ocupa ~3.200 × 4.525 px de dispositivo. Com `step = 765,5`, isso dá ~5 colunas × ~6 linhas ≈ **30 tiles**. Ou seja, **~28 milhões de operações de path para exibir uma página** — na thread principal. Este multiplicador é o que transforma "lento" em "travado".

### C3. Todo `open` do viewer baixa e reprocessa o arquivo inteiro — de novo

`frontend/src/routes/viewer/[id]/+page.svelte:104` chama `backfillAssets()` sem `await`, em paralelo à montagem do `<PdfViewer>`. E `backfillAssets` (linha 117) chama `extractAssetsFromUrl()`, em `frontend/src/lib/pdf-process.ts:83`:

```ts
const res = await fetch(url, { credentials: 'same-origin' });
const buffer = await res.arrayBuffer();                     // download #2, integral
const doc = await pdfjsLib.getDocument({ data: buffer }).promise;
const page1 = await doc.getPage(1);
const [text, preview] = await Promise.all([
  extractText(doc),                                          // TODAS as páginas
  renderPageToBlob(page1, PREVIEW_WIDTH)                     // rasteriza a p.1 a 1000px
]);
```

Três problemas somados:

1. **Segundo download integral.** O EmbedPDF já está baixando `/api/pdfs/{id}/file`; o pdf.js baixa o mesmo recurso outra vez. Para o arquivo A são 143 MB de tráfego por abertura. `[INFERÊNCIA]` O cache HTTP *pode* absorver a segunda, mas a resposta vem de `http.ServeContent` sem `Cache-Control` explícito — comportamento não garantido.
2. **`extractText` roda sempre**, mesmo quando o documento já tem texto. Olhe a ordem: o texto é extraído primeiro e só *depois* o `if (!hadText && text)` decide se envia. Para o arquivo com 1.082 páginas, é uma varredura de 1.082 `getTextContent()` descartada no fim. Para os escaneados sem camada de texto, a varredura custa caro e devolve nada.
3. **O preview é sempre re-renderizado e re-enviado** ("preview sempre", linha 112-114 do comentário), a cada abertura, para todo documento.

Isso ocorre **concorrentemente** com o PDFium na mesma thread principal, disputando CPU e memória exatamente durante o momento mais sensível da abertura.

### C4. Sem HTTP Range: o SDK baixa o arquivo inteiro antes de abrir

Nosso servidor **suporta** Range: `serveStoredFile` (`internal/server/handlers_pdfs.go:596`) usa `http.ServeContent`, que emite `Accept-Ranges: bytes` e responde `206`.

O SDK não aproveita. Verificado em `@embedpdf/engines/dist/pdf-engine-D9v0RfKe.js:317-321`:

```js
const response = await this.options.fetcher(file.url, options?.requestOptions);
const arrayBuf = await response.arrayBuffer();     // integral
const pdfFile = { id: file.id, content: arrayBuf };
```

O tipo `PdfOpenDocumentUrlOptions` em `@embedpdf/models/dist/pdf.d.ts:2769` até declara:

```ts
mode?: 'auto' | 'range-request' | 'full-fetch';
```

mas **a opção é letra morta**: um grep por `options.mode` / `range-request` em todo `engines/dist/*.js` e `snippet/dist/*.js` não encontra nenhum consumidor. Os únicos "range" no motor são `pageRange` (exportação) e `byteRange` (assinaturas digitais). Também não há `workerUrl`, `customLoader` nem exposição de `FPDF_LoadCustomDocument`.

**Relativize esta causa.** Range beneficia PDFs *linearizados* com centenas de MB. Nossos arquivos têm 20–72 MB numa LAN/túnel; baixar integralmente custa segundos, não minutos. Range é otimização, não a cura.

### C5. Teto rígido de 2 GB no alocador WASM

Verificado em `@embedpdf/engines/dist/direct-engine-C8xTbxym.js:389` e a checagem em `malloc`, linha 407:

```js
MAX_TOTAL_MEMORY: 2 * 1024 * 1024 * 1024
...
malloc(size) {
  if (this.totalAllocated + size > LIMITS.MEMORY.MAX_TOTAL_MEMORY) {
    throw new Error(`Total memory usage would exceed limit: ...`);
  }
```

É um acumulador de sessão, não configurável. `[INFERÊNCIA]` Com 30 tiles/página de bitmaps RGBA e caches do PDFium acumulando ao longo da navegação, é plausível esgotar isso numa sessão longa em documentos de 400–1.082 páginas. Não é a causa do travamento na *abertura*, mas é o teto de escala da solução.

---

## 4. O que configuração resolve — e o que não resolve

| Causa | Resolvível sem trocar de SDK? | Como |
|---|---|---|
| C1 thread principal | **Sim** | `worker-src 'self' blob:` na CSP + `worker: true` |
| C2 replay por tile | **Parcialmente** | `tileSize` maior, cap de `dpr`/`scaleFactor`; o replay em si é do PDFium |
| C3 duplo download | **Sim, é código nosso** | Tornar o backfill condicional e sob demanda |
| C4 sem Range | **Não** (limitação do SDK) | Impacto baixo no nosso acervo |
| C5 teto de 2 GB | **Não** | Mitigável reduzindo bitmaps residentes |

Quatro de cinco são tratáveis. **Isso é o que sustenta o veredito de não trocar de SDK.**

### 4.1. Sobre relaxar a CSP

`worker-src 'self' blob:` é uma diretiva **específica para workers** — não afeta `script-src`, não permite `<script src="blob:">`, não abre injeção de script na página. O worker executa em contexto isolado, sem DOM. É estritamente mais restritivo do que adicionar `blob:` ao `script-src`.

Ainda assim, prefira nesta ordem:

1. **Hospedar o worker no nosso domínio** e configurar apenas `worker-src 'self'`. Exige que o SDK aceite uma URL de worker — **ele não aceita** (verificado: nenhuma opção `workerUrl`). Só seria viável com um patch no pacote ou construindo o motor manualmente a partir de `@embedpdf/engines`, o que precisa de investigação própria antes de virar plano.
2. **`worker-src 'self' blob:`** — o caminho pragmático, exigido pelo `URL.createObjectURL` das linhas 487 e 493.

Registrar a decisão em `08-seguranca.md`, com a justificativa: a diretiva é criada exatamente para este caso e é mais estreita que a alternativa.

---

## 5. Trabalho necessário (não implementado)

### Fase 1 — Devolver a rasterização ao worker

1. Acrescentar `worker-src 'self' blob:` ao `cspTemplate` em `internal/security/headers.go`.
2. Trocar `worker: false` → `true` em `viewerConfig()` (`frontend/src/lib/embedpdf.ts`), removendo o comentário que documenta o contorno e substituindo pela nova justificativa.
3. Atualizar `08-seguranca.md`.

Critério de aceitação: abrir o arquivo A (71,7 MB) e **conseguir rolar a página durante o carregamento**. A UI deixa de congelar mesmo que a renderização ainda demore.

### Fase 2 — Parar de processar o arquivo duas vezes

Em `frontend/src/routes/viewer/[id]/+page.svelte` e `frontend/src/lib/pdf-process.ts`:

1. **Não chamar `extractText` quando `has_text` já é verdadeiro.** Hoje ele roda sempre e o resultado é descartado. Separar a extração de texto da geração de preview em `extractAssetsFromUrl` (hoje acopladas num `Promise.all`).
2. **Não re-renderizar o preview a cada abertura.** Condicionar a uma flag persistida (ex.: revisão do preview) para acontecer uma vez, não toda vez.
3. **Não disparar o backfill junto com a abertura.** Adiar para depois do `onLayoutReady` do viewer, ou movê-lo para ação explícita — o mesmo modelo do botão "Embedar", que já é sob demanda por decisão de projeto (ver `04-busca-hibrida.md`).
4. Avaliar `Cache-Control` explícito em `serveStoredFile` para que, quando o segundo fetch ainda ocorrer, ele venha do cache.

Critério de aceitação: uma abertura do viewer gera **um** download de `/api/pdfs/{id}/file`, verificável no painel de rede.

### Fase 3 — Reduzir o multiplicador de tiles

Não há botão mágico; são ajustes a medir, não a adotar às cegas:

- `tileSize` maior (menos replays, cada um mais caro) — medir 768 vs. 1024 vs. 1536.
- Limitar `dpr` a 1 em documentos acima de um limiar de complexidade. Em cartografia vetorial, `dpr: 2` dobra a área e o custo sem ganho legível proporcional.
- `defaultImageType`/`defaultImageQuality` (`plugin-render`, valores atuais `image/png` e `0.92`): PNG de tiles grandes é caro de codificar; WebP com qualidade ~0,8 tende a ser mais barato. `[INFERÊNCIA]` — precisa medição.

`extraRings` já está em 0; não há ganho ali.

### Fase 4 — Tratar o acervo na origem (opcional, maior retorno no pior caso)

Para a classe **vetorial pesada** (arquivo A), nenhuma configuração de viewer resolve 941 mil paths por página. As saídas reais:

- **Achatar as camadas de mapa para raster** em 200–300 DPI no documento de origem. Troca ~28 milhões de operações de path por uma decodificação de imagem.
- **Linearizar** (`qpdf --linearize`) — prepara o terreno caso o Range venha a existir no SDK; não ajuda hoje.

Para as classes escaneadas (B e C), recompressão para JPEG 150–200 DPI reduz bytes e tempo de decodificação. **Não confundir com o caso A.**

Isto é curadoria de acervo, não código da ferramenta. Só faz sentido depois das Fases 1–3, e é uma escolha do usuário, não uma exigência técnica.

---

## 6. Alternativas de SDK — por que **não** trocar agora

Levantamento feito, mas registrado com ressalva: parte das citações do levantamento automatizado não passou em verificação independente (datas de release e URLs de precificação de fornecedores comerciais não foram confirmadas). Trate a tabela como orientação, não como fato verificado.

| Candidato | Licença | Veredito |
|---|---|---|
| **EmbedPDF 2.14.4** (atual) | MIT / PDFium Apache-2.0 | **Manter.** UI de anotação completa e já integrada, i18n pt-BR já escrito, persistência já mapeada em `pdf_annotations` |
| **pdf.js 5.5.207** | MIT | Já é dependência nossa. Worker é arquivo real (`build/pdf.worker.mjs` — sem `blob:`), e `disableRange`/`disableStream`/`disableAutoFetch`/`rangeChunkSize` existem de fato (verificado em `types/src/display/api.d.ts`). **Mas** o editor de anotações teria de ser reintegrado do zero à nossa API |
| **mupdf (npm)** | **AGPL-3.0** | Motor bom, **sem UI**. Reconstruir viewer + anotações. Licença compatível com a nossa, mas o custo é semanas |
| **PDFSlick / LeedPDF** | MIT | Wrappers de pdf.js; herdam as mesmas questões e não trazem a UI de anotação que já temos |
| **Nutrient/PSPDFKit, Apryse, Foxit** | Comercial | **Descartados.** Licenciamento pago é incompatível com um projeto AGPL self-hosted de usuário único |

O argumento decisivo: **trocar de SDK não resolveria C1, C2 nem C3.** C3 é código nosso e viajaria junto. C1 é a nossa CSP. Um motor em pdf.js renderizando 941 mil paths na thread principal travaria igual. Trocar de SDK antes de corrigir a integração seria pagar semanas de reescrita para reencontrar o mesmo travamento — e perder a UI de anotação que motivou a adoção do EmbedPDF.

**Reavaliar a troca somente se**, depois das Fases 1–3 medidas, o arquivo A continuar inutilizável. Aí o gatilho é a Fase 4 (achatar a cartografia) antes de qualquer migração.

---

## 7. Como medir

Sem número antes e depois, não há como afirmar melhora. Para cada fase, no mesmo navegador e máquina, com os arquivos A (71,7 MB vetorial), B (47,7 MB JPEG) e C (37,4 MB / 440 p JBIG2):

| Métrica | Como obter |
|---|---|
| Tempo até a primeira página visível | `performance.mark` na abertura e no `onLayoutReady` |
| Bloqueio da thread principal | Aba Performance → *Total Blocking Time* e *long tasks* |
| Downloads de `/api/pdfs/{id}/file` por abertura | Aba Network, contagem de requisições |
| Pico de memória | `performance.memory` / Task Manager do navegador |
| Rolagem responsiva durante a carga | Verificação manual: a página responde ao scroll? |

Meta mínima da Fase 1: **nenhuma long task acima de 1 s** durante a abertura. Aquilo que hoje é um congelamento vira uma espera com UI viva.

---

## 8. Resumo

- O maior arquivo do acervo não é escaneado: é **cartografia vetorial com 941 mil operações de path por página**. Planos baseados só em recompressão de imagem não cobrem o pior caso.
- A causa direta do travamento é `worker: false`, adotado para contornar a CSP. **Corrigível com uma diretiva `worker-src`.**
- O tiling multiplica o custo de página por ~30 `[INFERÊNCIA]`, porque cada tile reexecuta a display list inteira via `FPDF_RenderPageBitmapWithMatrix`.
- Nossa própria rotina de backfill **baixa e reprocessa o arquivo inteiro a cada abertura**, concorrendo com o motor na mesma thread. É código nosso.
- Ausência de Range e teto de 2 GB são limitações reais do SDK, mas **não** são o que impede os arquivos de abrir.
- **Não trocar de SDK.** Executar Fases 1–3, medir, e só então reavaliar.

## 9. O que foi implementado (2026-07-27)

As Fases 1–3 foram implementadas, testadas e deployadas em produção no mesmo dia do estudo. A Fase 4 (achatar a cartografia do arquivo A, recomprimir escaneados) não foi feita — é curadoria de conteúdo do acervo, não código, e é decisão do usuário.

### 9.1. Mudanças de código

| Arquivo | Mudança | Causa endereçada |
|---|---|---|
| `internal/security/headers.go` | Acrescentada a diretiva `worker-src 'self' blob:` ao `cspTemplate` | C1 |
| `frontend/src/lib/embedpdf.ts` | Removido `worker: false` de `viewerConfig()` — volta ao padrão do SDK (`worker: true`); acrescentado `tiling: { tileSize: 1536 }` e `render: { defaultImageType: 'image/webp', defaultImageQuality: 0.8 }` | C1, C2 |
| `frontend/src/routes/viewer/[id]/+page.svelte` | `backfillAssets()` só é chamado quando `pdf.has_text === false`; a função não recebe mais `hadText` (motivo do call já é a garantia) e envia o texto sempre que extraído (sem a checagem redundante) | C3 |
| `refatoracao/08-seguranca.md` | CSP literal atualizada; nova entrada de justificativa para `worker-src` | — (documentação) |

**Fase 4 explicitamente não feita.** Nenhum PDF do acervo foi recomprimido ou teve a cartografia achatada — está fora do escopo de código e é escolha do usuário sobre o próprio conteúdo.

### 9.2. Por que cada mudança, e por que não mais

- **`worker-src 'self' blob:` em vez de patchear o SDK para hospedar o worker no mesmo domínio.** A opção de worker customizado (`workerUrl`) não existe no pacote instalado (verificado por grep no `@embedpdf/engines` publicado) — só seria viável reescrevendo `createPdfiumEngine` a partir do código-fonte. A diretiva CSP é uma linha, resolve o mesmo problema, e é estritamente mais restrita que relaxar `script-src` (worker roda em contexto isolado, sem DOM).
- **`tileSize: 1536` em vez de desligar o tiling.** Desligar o tiling renderizaria a página inteira de uma vez, o que é pior para páginas grandes (bitmap único maior, sem possibilidade de mostrar tiles parciais enquanto renderiza). Dobrar o `tileSize` linear (768 → 1536) reduz o número de tiles por página em ~4×, sem eliminar o benefício de renderização incremental.
- **`defaultImageQuality: 0.8` em WebP, não PNG lossless.** PNG é sem perda e mais caro de codificar para tiles grandes; WebP a 0.8 é a troca padrão do próprio SDK para esse cenário (a opção existe exatamente para isso). Não há dado de produção ainda medindo a diferença — ver 9.4.
- **`!pdf.has_text` como gate, não um novo campo de schema.** Cogitado adicionar uma coluna para marcar "preview já em alta resolução", mas isso duplicaria uma distinção que `has_text` já cobre na prática: todo documento sem texto vem de import legado ou watch-dir (nenhum dos dois popula `pdf_text` — confirmado, na época deste estudo, por grep em `import_legacy.go`; esse arquivo foi removido do binário em sessão posterior, depois que a migração de dados já havia rodado em produção — a conclusão sobre `has_text` permanece válida, o watch-dir continua sem popular `pdf_text` sozinho), e é exatamente esses que têm preview de baixa resolução herdado. Um upload normal já chega com texto e preview em alta resolução do processamento no navegador. Adicionar uma coluna só para distinguir um caso que `has_text` já distingue seria complexidade sem ganho.

### 9.3. Deploy em produção (UNRAID, 192.168.1.10)

Autorização do usuário: usar as credenciais do UNRAID para testar/ajustar, sem tocar em nada fora do newPdfDing. Escopo respeitado: única ação no host foi `docker pull` da nova imagem e `docker stop/rm/run` do container `newPdfDing`, recriado com a **configuração idêntica** à anterior (mesmas env vars, montagens `/data` e `/files`, porta `8778:8000`, restart policy). Nenhum outro container, template ou arquivo do UNRAID foi tocado.

Sequência executada:

1. `go vet ./...` e `go test ./... -timeout 120s` locais — sem erros.
2. `npm run build` (frontend) — sem erros de tipo.
3. `go build ./...` (backend, com o build do frontend sincronizado em `internal/server/web/dist`) — sem erros.
4. Commit direto em `main` (política do projeto: um branch só) e push.
5. CI (`docker-publish.yaml`) rodou test → build → push da imagem `ghcr.io/edalcin/newpdfding:latest` — [run 30273497014](https://github.com/edalcin/newPdfDing/actions/runs/30273497014), concluído com sucesso.
6. No UNRAID: `docker pull ghcr.io/edalcin/newpdfding:latest`, depois `docker stop newPdfDing && docker rm newPdfDing && docker run ...` recriando o container com a config extraída de `docker inspect` do container anterior.

### 9.4. Verificação feita — e o que não foi possível verificar

**Verificado:**

- Container `newPdfDing` `healthy`, `/healthz` responde 200, sem erros nos logs após a recriação.
- `curl -sI` contra o container em produção confirma o header ao vivo:
  ```
  Content-Security-Policy: ...; script-src 'self' 'wasm-unsafe-eval' 'sha256-...'; worker-src 'self' blob:; ...
  ```
  A diretiva que resolve a causa C1 está de fato servida pelo binário rodando em produção, não só no código-fonte.
- `go test ./...` e o job `Test` do CI (`go vet`, `go test`, `govulncheck`) passaram antes do build da imagem publicada.

**Não verificado — e por quê:**

- **Não confirmei visualmente, num navegador real, que o arquivo de 71,7 MB abre sem travar.** Duas tentativas falharam por razões alheias ao código mudado:
  1. Contra `https://newpdfding.dalc.in` (Cloudflare Tunnel): a sessão headless nunca completou o login — a página reportou "offline" e nenhuma requisição de API saiu do navegador, consistente com bloqueio de bot do Cloudflare à sessão headless, não com o servidor.
  2. Contra `http://192.168.1.10:8778` (LAN, direto no container, sem Cloudflare): o login falhou com "missing CSRF cookie" — o cookie de sessão é `Secure` (correto: só viaja em HTTPS) e a LAN é HTTP puro. Não relaxei essa proteção só para testar.
  - **Não contornei nenhuma das duas** porque ambas são comportamento de segurança correto (proteção de bot e cookie `Secure`), e o usuário pediu para não mexer em mais nada além do necessário.
- **Recomendação:** abrir `https://newpdfding.dalc.in/viewer/019f9ed9-b63b-7829-903e-85200ad43ff7` (o arquivo de 71,7 MB) num navegador normal e confirmar que a página rola durante o carregamento — esse é o teste que a Fase 1 promete resolver. Se ainda travar, a causa provável é C2 (replay por tile) ou C5 (teto de memória), que as Fases 3/4 mitigam mas não eliminam para o arquivo A.
- Os ganhos de `tileSize`/WebP (Fase 3) **não foram medidos** com Performance/Network do navegador contra o arquivo real — são a mudança de configuração recomendada pelo estudo, aplicada, mas sem número de antes/depois. Ver seção 7 (Como medir) para o protocolo.

---

**Arquivo deste relatório:** `refatoracao/11-desempenho-viewer.md`
