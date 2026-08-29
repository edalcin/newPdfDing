# Não existe embedding em massa

`POST /api/admin/reembed` enfileirava de uma vez todos os documentos sem vetor atual. Disparado sobre 160+ documentos, derrubou o host UNRAID: cada documento sem texto extraído copia o PDF inteiro para um arquivo temporário e roda o parser, e em UNRAID o `/tmp` do container é RAM do próprio sistema operacional. O endpoint, a fila em lote (`enqueueBulk`) e o botão em `/admin` foram **removidos**, sem substituto: documentos são embedados um por vez, pelo ícone de cérebro no card, por ação explícita do usuário.

## Consequences

Encher um acervo grande é trabalho manual e demorado. Isso é aceito: a busca léxica (FTS5) continua funcionando sobre todo o acervo enquanto os vetores não existem, então o sistema permanece útil durante todo o processo — a busca semântica degrada aos poucos em vez de o servidor cair de uma vez.

As correções de memória feitas em conjunto (temporário em disco em vez de RAM, teto de 2 MB no texto extraído, `mem_limit: 1g` no container) tornam o caminho de extração seguro por documento. Um lote poderia, tecnicamente, voltar a existir sobre elas. Não volta: a ação em massa não acrescenta capacidade nenhuma que os cliques individuais não tenham, e acrescenta um único botão capaz de multiplicar qualquer defeito futuro do caminho de extração por todo o acervo.

Consequência aceita junto: não há filtro por estado de embedding na listagem, então achar os documentos sem vetor é rolar a lista procurando o ícone âmbar. Se isso incomodar na prática, o filtro é `WHERE NOT EXISTS (SELECT 1 FROM pdf_embeddings WHERE pdf_id = pdfs.id)` — SQL puro, compatível com a paginação por cursor, e não precisa do estado derivado em Go.

**Consequência posterior**: a decisão continua valendo — um documento por acionamento, sem lote. O acionamento passou a ser assíncrono (`POST .../embed` responde `202` e só enfileira; um worker único em `internal/server/embedqueue.go` serializa as chamadas ao modelo), o que não reintroduz o lote, e o filtro por estado cogitado acima foi implementado (`GET /api/pdfs?embedding=none|current|stale`).
