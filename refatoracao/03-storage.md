# Storage

## Decisão

O acervo de PDFs mora **exclusivamente no filesystem local**, sob o caminho externo apontado por `FILES`. Não existe backend S3 para os arquivos do acervo, não existe comutação de backend em tempo de execução, e não existe tela de migração de storage na interface (decisão 3 de [Visão geral](00-visao-geral.md)).

O único ponto do produto que fala com S3/MinIO é o **destino do job de backup** — funcionalidade já existente hoje, preservada sem mudança de escopo, com bucket e credenciais próprios em variáveis `BACKUP_*`. Backup é cópia de segurança, não é o storage do acervo.

## Esquema de chaves

Toda chave é relativa à raiz `FILES`. O identificador usado na chave é sempre o `pdf_id` (UUIDv7), nunca um nome derivado do título do arquivo.

| Tipo de arquivo | Esquema de chave |
|---|---|
| PDF | `{collection_id}/pdf/{file_directory/}{pdf_id}.pdf` |
| Thumbnail | `{collection_id}/thumb/{pdf_id}.png` |
| Preview | `{collection_id}/preview/{pdf_id}.png` |

`{file_directory/}` é o subdiretório lógico opcional (`pdfs.file_directory`, ver [Modelo de dados](02-modelo-de-dados.md)); quando vazio, o PDF fica direto sob `{collection_id}/pdf/`.

**Por que `pdf_id` e não um nome slugificado**, como o produto atual faz: chave por `pdf_id` elimina a renomeação em disco toda vez que o PDF é renomeado (o registro no banco muda, o arquivo em disco não precisa mudar), elimina colisão de nomes entre PDFs com títulos iguais, e elimina o sufixo UUID de desambiguação que o código atual precisa gerar para resolver essa colisão.

## `storage.Backend` e `LocalBackend`

A assinatura exata da interface `Backend` (com `Put`, `Get`, `Delete`, `List`, `Name`) e da interface complementar `Seeker` (com `OpenSeek`) está definida em [Arquitetura](01-arquitetura.md) — não repetida aqui.

Existe **uma única implementação**, `LocalBackend`, com raiz fixa em `FILES`. A interface não existe para permitir troca de backend no futuro imediato; existe para concentrar num só lugar a validação de caminho e o acesso posicional (`Range`) que, de outra forma, vazariam para os handlers HTTP.

Comportamento de `LocalBackend`:

- **`Put`** — `os.MkdirAll` do diretório-pai da chave, depois `os.Create` + `io.Copy` do corpo recebido.
- **`Get`** — `os.Open` + `Stat`, devolvendo o `io.ReadCloser` e o tamanho.
- **`OpenSeek`** — devolve o próprio `*os.File` como `io.ReadSeekCloser`, para que o chamador sirva requisições `Range` sem buffer intermediário.
- **`Delete`** — remove o arquivo e, em seguida, sobe removendo os diretórios-pai que ficaram vazios, um nível de cada vez, parando ao alcançar a raiz `FILES` (nunca remove a raiz nem sobe além dela).

## Proteção contra path traversal

Toda chave passa pelo mesmo caminho de validação antes de qualquer I/O:

1. Montar o caminho absoluto com `filepath.Join(root, key)`.
2. Aplicar `filepath.Clean` ao resultado.
3. Recusar com erro se o caminho limpo não tiver `root` como prefixo.

Isso rejeita, antes de tocar o disco: chaves contendo `..`, chaves absolutas, e chaves usando o separador de caminho do Windows (`\`) — mesmo rodando a imagem em Linux, a validação não confia no separador vindo da chave.

## Entrega de arquivos ao browser

Toda rota de download/stream (`GET /api/pdfs/{id}/file`, `/thumbnail`, `/preview`, `/download`, e as equivalentes públicas de compartilhamento em [API](05-api.md)) usa `http.ServeContent` sobre o `io.ReadSeekCloser` devolvido por `OpenSeek`. Isso dá suporte a `Range` e resposta `206 Partial Content` de graça, via a biblioteca padrão — sem código de range manual.

Isso **não é opcional**: o viewer pdf.js depende de requisições parciais para abrir PDFs grandes sem baixar o arquivo inteiro de uma vez.

## Falhas

| Cenário | Resposta | Comportamento adicional |
|---|---|---|
| Arquivo ausente em disco, mas linha presente no banco | `404` | Log de aviso nomeando `pdf_id` e a chave que faltou |
| Disco cheio durante `Put` (upload) | `507` | A linha do PDF **não** é inserida no banco |

O upload grava o arquivo em disco antes de fazer commit da transação SQL; se o commit falhar por qualquer motivo após o arquivo já estar gravado, o arquivo é apagado — nunca fica um arquivo órfão em disco sem linha correspondente no banco.

## Backup em S3/MinIO — o único uso de S3 no produto

Funcionalidade já existente hoje, preservada sem mudança: job periódico, executado a cada `BACKUP_INTERVAL_HOURS`, usando **AWS SDK Go v2** (ver dependências em [Arquitetura](01-arquitetura.md)). Bucket e credenciais são próprios do backup, em variáveis `BACKUP_*` (lista completa em [Docker, CI e Deploy](07-docker-ci-deploy.md)) — não têm relação com o storage do acervo.

- **O que é enviado**: o arquivo do banco SQLite (`DB_PATH`) e todos os arquivos sob `FILES`.
- **Deduplicação de envio**: comparação por nome + tamanho contra o que já está no bucket, para não reenviar o que já foi copiado.
- **Criptografia opcional** (`BACKUP_ENCRYPTION_ENABLE`): AES-256-GCM, com chave derivada por `scrypt` a partir de `BACKUP_ENCRYPTION_PASSWORD` e um salt aleatório gravado no próprio cabeçalho do arquivo cifrado — substitui o Fernet usado hoje pelo Python. **Backups cifrados com Fernet pela versão antiga não são legíveis pela versão nova**; se houver backups Fernet existentes, devem ser restaurados com a versão antiga do produto antes de migrar para esta.
- **Compatibilidade com MinIO**: via `BACKUP_ENDPOINT`, usando `BaseEndpoint` do SDK com `UsePathStyle: true`.
- **Restauração**: flag de CLI `-restore-backup`, que baixa tudo do bucket, decifra (se cifrado) e sobrescreve `DB_PATH` e `FILES` — exige confirmação interativa antes de executar, por ser destrutiva.

## Ponto de extensão (não implementado agora)

O objetivo original de "adotar a mesma estratégia de storage em Amazon S3 do `pkd`" foi retirado do escopo desta refatoração: os arquivos do acervo ficam no filesystem sob `FILES`, e S3 permanece só como destino do backup.

Se um dia for necessário reintroduzir armazenamento remoto para o acervo, o ponto de extensão já existe: a interface `storage.Backend`. Bastaria uma segunda implementação da interface voltada a armazenamento remoto e um mecanismo de escolha de qual backend usar. Nenhuma outra parte deste plano precisaria mudar, e **nenhuma alteração de schema seria necessária** — `pdfs.storage_key` (ver [Modelo de dados](02-modelo-de-dados.md)) já é relativo e independente de backend.
