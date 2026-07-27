# Storage

## Decisão

O acervo de PDFs mora **exclusivamente no filesystem local**, sob o caminho externo apontado por `FILES`. Não existe backend S3 para os arquivos do acervo, não existe comutação de backend em tempo de execução, e não existe tela de migração de storage na interface (decisão 3 de [Visão geral](00-visao-geral.md)). O produto **não tem nenhuma integração com Amazon S3, MinIO ou qualquer outro object storage** — nem para o acervo, nem para backup.

## Esquema de chaves

Toda chave é relativa à raiz `FILES`. O identificador usado na chave é sempre o `pdf_id` (UUIDv7), nunca um nome derivado do título do arquivo.

| Tipo de arquivo | Esquema de chave |
|---|---|
| PDF | `pdf/{pdf_id}.pdf` |
| Preview | `preview/{pdf_id}.png` |

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

## Backup — removido desta refatoração

O produto atual tem um job de backup periódico que envia o banco SQLite e os arquivos do acervo para um bucket S3/MinIO. Essa funcionalidade **foi removida do escopo desta refatoração**, por decisão do usuário de eliminar toda integração com Amazon S3 do produto — ver [Funcionalidades intencionalmente removidas](10-inventario-funcionalidades.md). Não há job de backup, criptografia de backup, nem comando de restauração via CLI nesta versão.

Backup, se necessário, é responsabilidade do operador do host — por exemplo, um `cron` ou `rsync` externo copiando periodicamente `DB_PATH` e o conteúdo de `FILES` para outro destino. Isso está fora do escopo do binário `newpdfding`.
