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

**Reindexar FTS5**:
Reconstruir o índice léxico a partir dos dados já guardados. Operação local, sem chamada externa, sem relação com embedding.
_Avoid_: reindexar (sozinho, é ambíguo com embedar)
