# Limpeza do repositório

Lista fechada e exaustiva do que é apagado, agrupado por categoria, cada grupo com o motivo da remoção. O implementador da [`ETAPA-0-LIMPEZA`](ETAPAS.md) executa esta lista sem julgar — nenhum item é opcional e nenhum item novo é adicionado sem atualizar este documento primeiro.

## O que é apagado

### Aplicação Python inteira

Motivo: todo o backend Django é substituído pelo backend Go descrito em [Arquitetura](01-arquitetura.md); nada do código Python sobrevive à refatoração.

- `pdfding/` — apagar por completo, incluindo todos os subdiretórios:
  - `pdfding/admin/`
  - `pdfding/backup/`
  - `pdfding/base/`
  - `pdfding/core/`
  - `pdfding/db/`
  - `pdfding/e2e/`
  - `pdfding/locale/`
  - `pdfding/media/`
  - `pdfding/pdf/`
  - `pdfding/static/`
  - `pdfding/templates/`
  - `pdfding/users/`
  - `pdfding/manage.py`
  - `pdfding/conftest.py`

### Empacotamento Python

Motivo: sem código Python, não há mais dependências Python a gerenciar nem hooks de qualidade Python a rodar.

- `pyproject.toml`
- `poetry.lock`
- `uv.lock`
- `setup.cfg`
- `.pre-commit-config.yaml`
- `.pytest_cache/`
- `.venv/`

### Runtime antigo

Motivo: o processo único Go substitui gunicorn + supervisord + huey; não há mais múltiplos processos a orquestrar nem variantes de banco de dados a escolher no compose.

- `supervisord.conf`
- `bootstrap.sh`
- `compose/postgres.docker-compose.yaml`
- `compose/sqlite.docker-compose.yaml`

Substituídos por um único `compose.yaml` de desenvolvimento (ver [Docker, CI e Deploy](07-docker-ci-deploy.md)).

### Frontend antigo

Motivo: o novo frontend SvelteKit vive isolado sob `frontend/`, com seu próprio `package.json` e árvore de dependências (ver [Frontend](06-frontend.md)); os artefatos Node da raiz do repositório antigo não têm mais função.

- `package.json` (da raiz)
- `package-lock.json` (da raiz)
- `node_modules/` (da raiz)

### Artefatos

Motivo: subprodutos de ferramentas de análise/assistente que não fazem parte do produto entregue.

- `graphify-out/`
- `.claude/` — **confirmar antes de apagar.** É ferramenta pessoal do dono do repositório, não herança do projeto upstream `mrmn2/PdfDing`. Só remover com confirmação explícita; se a resposta for não, remover este item da lista de execução da `ETAPA-0-LIMPEZA` antes de rodar a etapa.

## Reescritos, não apagados

Estes arquivos permanecem no repositório, mas seu conteúdo é totalmente substituído pelo conteúdo alvo descrito nos demais documentos desta pasta — não são cópias herdadas do projeto Django:

- `Dockerfile` — conteúdo alvo em [Docker, CI e Deploy](07-docker-ci-deploy.md).
- `README.md` — conteúdo alvo no [índice desta pasta](README.md) mais a linha de atribuição AGPL descrita abaixo.
- `.github/workflows/docker-publish.yaml` — conteúdo alvo em [Docker, CI e Deploy](07-docker-ci-deploy.md).
- `.gitignore`
- `.dockerignore` — conteúdo alvo em [Docker, CI e Deploy](07-docker-ci-deploy.md).
- `CHANGELOG.md`
- `SECURITY.md`

## Preservado intacto

- `LICENSE` — mantido sem alteração (ver decisão de licenciamento abaixo).

## Decisão de licenciamento

O projeto é derivado de `mrmn2/PdfDing`, licenciado sob **AGPL-3.0**. O briefing pede independência do projeto original quanto a código e a interface — não quanto à licença. Mesmo com a reescrita total do backend em Go e do frontend em SvelteKit, este plano mantém:

- o arquivo `LICENSE` com o texto AGPL-3.0 inalterado;
- uma linha de atribuição ao projeto original no `README.md`.

Esta é a postura juridicamente segura enquanto o produto continuar a embutir o pdf.js herdado e a herança de design (fluxos, telas, nomes de funcionalidades) do projeto original. A alternativa — obter parecer jurídico próprio para justificar uma relicenciação — é apontada aqui como possível, mas **não é recomendada** neste plano: não há necessidade de negócio que justifique o custo e o risco de uma reavaliação de licenciamento nesta refatoração.

## Varredura de resíduos

Depois de executar a limpeza, rodar os quatro comandos abaixo a partir da raiz do repositório (`D:/git/newPdfDing`) para provar que nada do stack antigo restou:

```bash
grep -ri "mrmn" .
grep -ri "pdfding\." .
grep -ri "django" .
grep -ri "huey\|supervisor\|gunicorn\|poetry" .
```

**Resultado esperado**: as únicas ocorrências permitidas são:

- referências dentro da pasta `refatoracao/` (este documento e os demais, que citam o projeto original e o stack antigo com propósito descritivo/histórico);
- a linha de atribuição ao projeto original AGPL-3.0 no `README.md` da raiz.

Qualquer outra ocorrência indica um resíduo do stack Django/Python que a `ETAPA-0-LIMPEZA` não removeu e deve ser corrigida antes de prosseguir para a `ETAPA-1-FUNDACAO`.
