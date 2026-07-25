# Changelog

Todas as mudanças notáveis deste projeto são documentadas aqui.

## [Unreleased]

### Changed

- Início da refatoração completa do stack: backend Python legado + HTMX → Go + SvelteKit. Plano de execução em [`refatoracao/`](refatoracao/README.md).
- `ETAPA-0-LIMPEZA`: remoção integral do stack Python legado (aplicação web, empacotamento de dependências, runtime multi-processo antigo, frontend Node da raiz, artefatos `graphify-out/`). Ver [`refatoracao/09-limpeza-repositorio.md`](refatoracao/09-limpeza-repositorio.md).
