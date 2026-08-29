# newPdfDing

Biblioteca pessoal de PDFs de um único usuário: guarda os arquivos, os metadados e as anotações, e permite achar um documento por palavra ou por sentido.

## Language

### Documento e conteúdo

**PDF**:
Um documento da biblioteca — o arquivo mais seus metadados, tags e anotações.
_Avoid_: arquivo, upload, item

**Texto extraído**:
O texto legível retirado de dentro de um PDF, guardado à parte do arquivo. Não depende de modelo de IA nenhum: uma vez extraído, vale para sempre.
_Avoid_: OCR, corpo, body

**Anotação**:
Um comentário do usuário ancorado em uma página de um PDF.
_Avoid_: nota, highlight, marcação

### Busca

**Busca léxica**:
A metade da busca que casa palavras, servida pelo índice FTS5. Funciona sem chave de API e sem vetor nenhum.
_Avoid_: busca sintática, full-text, keyword search

**Busca semântica**:
A metade da busca que casa sentido, comparando o vetor da consulta com o vetor de cada documento. Exige `GEMINI_API_KEY` e só alcança documentos já embedados.
_Avoid_: busca vetorial, busca por similaridade, KNN

**Busca híbrida**:
A busca do produto: as duas metades acima combinadas em um único resultado ordenado.

### Embedding

**Embedar**:
Calcular o vetor de um documento a partir do seu texto extraído e gravá-lo. Acontece um documento por vez, por ação explícita do usuário no botão do card.
_Avoid_: indexar, vetorizar, processar

**Modelo de embedding**:
O modelo que transforma texto em vetor. É **fixo no código** (`config.EmbedModel`), não configurável: trocá-lo invalida todo vetor já gravado, porque espaços vetoriais de modelos diferentes não são comparáveis.
_Avoid_: modelo de IA (esse termo cobre também o modelo de texto, que é outro e é escolhível)

**Estado de embedding**:
Em que situação está o vetor de um documento. Três valores, derivados na leitura, nunca guardados como coluna:
- **nenhum** (`none`): não existe vetor. É o estado de um documento recém-importado, e o único ponto de partida para embedar.
- **atual** (`current`): existe vetor e ele corresponde ao texto de hoje.
- **desatualizado** (`stale`): existe vetor, mas o texto do documento mudou desde que ele foi calculado.

_Avoid_: "pendente" como termo do domínio — junta `nenhum` e `desatualizado` em um só nome e esconde que são situações diferentes.

**Job de embedding**:
O acompanhamento de uma chamada a `embedar` enquanto ela está em curso, com estado próprio e independente do estado de embedding do documento. Passa por três fases não terminais — `queued` (na fila, aguardando o worker), `extracting` (lendo o texto extraído), `embedding` (chamando a API do modelo) — até terminar em `done` ou `failed`. Exposto por `GET /api/embed/jobs` como um mapa `pdf_id -> {state, error}`; um job `done` some do mapa 60s depois, um `failed` fica até ser reenfileirado.
_Avoid_: tarefa, processo, task

**Fila de embedding**:
O worker único de background que drena os jobs de embedding em série — nunca duas chamadas ao modelo rodam ao mesmo tempo, mesmo que vários documentos sejam acionados em sequência. `POST /api/pdfs/{id}/embed` apenas enfileira (responde `202`); quem embeda de fato é o worker.
_Avoid_: fila de processamento, background job (sozinho, sem dizer que é serial)

**Filtro por estado de embedding**:
O parâmetro `embedding=none|current|stale` de `GET /api/pdfs`, e os três chips correspondentes na biblioteca. Filtra pelo estado de embedding definido acima — não pelo estado do job.

**Reindexar FTS5**:
Reconstruir o índice léxico a partir dos dados já guardados. Operação local, sem chamada externa, sem relação com embedding.
_Avoid_: reindexar (sozinho, é ambíguo com embedar)
