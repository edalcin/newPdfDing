# Modelo de embedding fixo no código

O modelo era escolhível em Configurações → IA (`settings['ai.embed_model']`, com fallback na variável de ambiente `EMBED_MODEL`), e trocá-lo pela interface marcou os 117 vetores do acervo como desatualizados de uma vez — o que levou o usuário a disparar o reembedding em massa que derrubou o servidor UNRAID. O modelo passa a ser a constante `config.EmbedModel = "models/gemini-embedding-2"`: a chave de configuração e a variável de ambiente foram removidas, e a área de administração apenas **exibe** o nome do modelo.

## Considered Options

- **Manter `EMBED_MODEL` como variável de ambiente, só mudando o default.** Rejeitado: apenas move o mesmo botão de auto-sabotagem do navegador para o template do UNRAID.
- **Constante no código com a variável de ambiente como escape hatch não documentada.** Rejeitado: comportamento que existe e ninguém conhece é o que se descobre às 3 da manhã.

## Consequences

Trocar de modelo agora exige um commit e um deploy. Isso é deliberado e proporcional: a troca invalida todo vetor gravado, obriga a reembedar o acervo inteiro à mão e deixa a busca semântica cega até o fim do processo. É uma decisão de projeto, não uma preferência de usuário.

`gemini-embedding-2` produz vetores de 3072 dimensões, o mesmo tamanho de `gemini-embedding-001`, mas seus espaços vetoriais **não** são compatíveis. A guarda de dimensão em `dotProduct` não detecta essa incompatibilidade — os tamanhos coincidem —, então os vetores antigos foram apagados do banco de produção em vez de mantidos: um vetor de `-001` comparado com uma consulta de `-2` produziria pontuação sem significado, capaz de passar do piso de similaridade.
