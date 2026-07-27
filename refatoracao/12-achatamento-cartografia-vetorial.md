# 12 — Achatamento de cartografia vetorial (Fase 4 do estudo de desempenho)

**Status:** guia prático, não implementado. Nenhum PDF do acervo foi modificado.
**Data:** 2026-07-27.
**Contexto:** complementa [11-desempenho-viewer.md](11-desempenho-viewer.md), seção 5 ("Fase 4 — Tratar o acervo na origem"), que recomendava esta ação mas não a detalhava.

---

## 1. O que é "achatar" e por que este arquivo específico precisa disso

"Achatar" (*flatten* / *rasterize*) significa **substituir o conteúdo vetorial de uma página por uma única imagem** do resultado já renderizado daquela página. A página deixa de conter formas geométricas (curvas de Bézier, preenchimentos, grupos de transparência) e passa a conter um bitmap — do mesmo jeito que uma foto.

Isto NÃO é o mesmo problema de "PDF escaneado com imagem grande". O relatório 11 identificou, medindo o maior arquivo do acervo (`019f9ed9-b63b-7829-903e-85200ad43ff7.pdf`, 71,7 MB / 108 páginas, "Plano de Gestão Territorial e Ambiental"), que ele **não é um escaneado**: 65,7 MB dos seus 71,7 MB são streams `FlateDecode` contendo operadores de desenho, não pixels. Descomprimindo o maior objeto:

```
obj 371: 12,5 MB comprimido -> 39,7 MB descomprimido
  853.998 curvas de Bézier + 62.715 movimentos + 62.612 preenchimentos
  = 941.063 operações de path NUMA ÚNICA PÁGINA
  marcador /PlacedPDF — cartografia (mapa) vetorial colada de Illustrator/GIS
```

Cada vez que essa página precisa aparecer na tela — inclusive uma vez **por tile** do EmbedPDF, ver 11-desempenho-viewer.md causa C2 — o motor PDFium reexecuta essas 941 mil operações do zero. Não existe cache de "já desenhei essa curva". **Nenhuma configuração de viewer resolve isso**, porque o custo está no conteúdo do arquivo, não em como ele é exibido.

Achatar troca esse custo por outro, muito mais barato: decodificar uma imagem já pronta. Uma página inteira com centenas de milhares de curvas vira uma única operação de "desenhar esta imagem aqui" — o mesmo custo, aproximadamente, de exibir uma foto de mapa.

---

## 2. O que se perde, e por que não importa aqui

| Perde | Por quê tudo bem neste caso |
|---|---|
| Texto selecionável/copiável **dentro do PDF** | A busca da aplicação não lê o texto do PDF em tempo real — ela lê a tabela `pdf_text`, extraída uma vez e já gravada no banco (ver [04-busca-hibrida.md](04-busca-hibrida.md)). Achatar a página não apaga essa cópia; a busca continua funcionando exatamente igual. Só se perde "selecionar e copiar uma frase" clicando diretamente no PDF renderizado. |
| Nitidez infinita ao dar zoom (vetor escala sem perda) | Um mapa exibido em tela ou impresso raramente precisa de zoom além do que a resolução de digitalização (DPI) escolhida já cobre — ver seção 3 para escolher o DPI certo. |
| Tamanho do arquivo pode não cair tanto quanto um scan | Streams vetoriais já são comprimidos (FlateDecode com ~3× de razão, medido). A imagem rasterizada pode ficar de tamanho parecido ou até maior se o DPI for alto demais — por isso o DPI é a decisão central, não um detalhe. |

**O que não se perde:** anotações (destaques/comentários) já salvas na tabela `pdf_annotations` — desde que a geometria da página (largura/altura em pontos) permaneça idêntica após o achatamento. Ver seção 5, "Risco de anotações existentes".

---

## 3. Escolher o DPI certo

DPI (pontos por polegada) controla a resolução do bitmap resultante. É a única decisão de qualidade nesta operação.

| DPI | Uso recomendado | Efeito |
|---|---|---|
| 150 | Texto/tabelas escaneados, leitura em tela | Legível em tela, ilegível impresso em detalhe |
| **200–250** | **Mapas e cartografia — recomendado para este caso** | Preserva rótulos de mapa, linhas finas de contorno, legendas pequenas |
| 300 | Documentos que serão impressos ou têm texto miúdo denso | Padrão de qualidade "impressão"; arquivo maior |
| 400+ | Raramente necessário | Ganho marginal, custo alto de tamanho e tempo de rasterização |

Para o arquivo A especificamente (mapas de gestão territorial com legendas e toponímia), **200 DPI é o ponto de partida recomendado**. Abaixo disso, nomes de municípios e linhas de divisa em mapas costumam ficar ilegíveis.

---

## 4. Como fazer — passo a passo

### 4.1. Ferramentas

Duas ferramentas, nenhuma precisa ficar instalada permanentemente:

- **Ghostscript** (`gs`) — rasteriza cada página em uma imagem PNG na resolução escolhida.
- **img2pdf** — remonta as imagens em um PDF, preservando o tamanho físico exato de cada página (lê o DPI embutido no PNG e calcula o tamanho em pontos automaticamente — é o que garante que a geometria não muda, ver seção 5).

Ambas rodam num contêiner Docker descartável (`--rm`), sem instalar nada no host — nem no Windows nem no UNRAID:

```bash
docker run --rm -v "<pasta-local>:/work" -w /work python:3.12-slim bash -c "
  apt-get update -qq && apt-get install -y -qq ghostscript >/dev/null &&
  pip install -q img2pdf &&
  gs -dNOPAUSE -dBATCH -sDEVICE=png16m -r200 -o page-%04d.png entrada.pdf &&
  img2pdf page-*.png -o saida.pdf
"
```

Troque `-r200` pelo DPI escolhido (seção 3). `entrada.pdf` e `saida.pdf` ficam em `<pasta-local>` no host, mapeada para `/work` no contêiner.

### 4.2. Obter o arquivo original

Duas formas, sem precisar de acesso SSH ao UNRAID:

- **Pela interface**: abrir o PDF em `/pdf/{id}` e usar "Baixar".
- **Pelo compartilhamento SMB** (já configurado no UNRAID — ver sessão anterior): a pasta `newpdfding/files/pdf/<storage_key>.pdf` está acessível como unidade de rede no Windows.

**Sempre guarde o arquivo original antes de mexer.** Esta operação não tem desfazer automático — se o achatamento sair ruim (DPI baixo demais, página faltando), a única forma de recuperar é reenviar o original de novo.

### 4.3. Rasterizar e remontar

Rodar o comando da seção 4.1 aponta para o arquivo baixado. Ao final, `saida.pdf` é o PDF achatado.

### 4.4. Verificar antes de reenviar

Três checagens obrigatórias, nesta ordem:

1. **Contagem de páginas idêntica.** `gs -q -dNODISPLAY -c "(saida.pdf) (r) file runpdfbegin pdfpagecount = quit"` (ou abrir e contar visualmente). Se o número mudar, **não prossiga** — algo saiu errado na rasterização.
2. **Tamanho de página idêntico ao original**, em pontos, para pelo menos a primeira e a última página (ver seção 5 — é o que protege anotações existentes). `pdfinfo saida.pdf` e `pdfinfo entrada.pdf` (ou equivalente) devem mostrar o mesmo `Page size`.
3. **Inspeção visual** de 3–4 páginas representativas, incluindo pelo menos uma com o mapa/legenda mais denso, na resolução final. Rótulos pequenos devem continuar legíveis.

### 4.5. Reenviar

O backend já expõe exatamente esta operação — substituir o conteúdo de um PDF existente mantendo tudo mais (tags, coleção, anotações, contagem de visualizações, data de criação): `PUT /api/pdfs/{id}/file` (ver [05-api.md](05-api.md)). Incrementa a `revision` e recalcula o hash/tamanho; não mexe em `num_pages`, `has_text` nem `preview_key` (por isso a checagem 4.4.1 é obrigatória — o app não vai avisar sozinho se a contagem de páginas mudou).

Não há botão na interface para isso hoje (ver 4.6) — o caminho é a API diretamente, autenticado com o cookie de sessão do navegador já logado:

```bash
curl -X PUT "https://newpdfding.dalc.in/api/pdfs/<id>/file" \
  -H "Cookie: <cookie de sessão copiado do navegador>" \
  --data-binary @saida.pdf \
  -H "Content-Type: application/pdf"
```

### 4.6. Nota: não existe UI para isso ainda

`PUT .../file` está implementado no servidor mas nenhum componente do frontend o chama hoje — foi verificado por busca no código-fonte do frontend. Se este fluxo (achatar cartografia, ou qualquer "substituir arquivo mantendo o registro") for usado com frequência, vale considerar adicionar um botão "Substituir arquivo" em `/pdf/{id}` num momento futuro. Fora de escopo deste documento, que é só o procedimento manual.

---

## 5. Risco de anotações existentes

Anotações (`pdf_annotations`) guardam retângulos em **coordenadas de página, em pontos**, geradas pelo EmbedPDF a partir do tamanho da página no momento da criação. Se o achatamento mudar o tamanho físico de uma página — mesmo por poucos pontos, por arredondamento de DPI — os destaques/comentários salvos ficam desalinhados na próxima abertura.

`img2pdf` (recomendado na seção 4.1) evita isso: ele lê a resolução (DPI) gravada no PNG pelo Ghostscript e calcula o tamanho de página em pontos a partir dela, reproduzindo o tamanho original com precisão de sub-pixel — desde que o DPI usado na rasterização (`-r200` no `gs`) seja o mesmo valor considerado pelo `img2pdf` (comportamento padrão, sem flags extras).

**Antes de achatar um PDF que já tem destaques ou comentários salvos**, confirme em `/highlights` ou `/comments` filtrando por aquele documento. Se houver anotações, faça a checagem 4.4.2 (tamanho de página) com mais rigor — comparar em pelo menos 3 páginas, não só a primeira.

---

## 6. Como identificar outros candidatos no acervo

O mesmo diagnóstico usado no relatório 11 (perfilar os streams internos do PDF) serve para varrer o acervo inteiro e achar outros arquivos "vetoriais pesados" — que parecem grandes mas não são escaneados, então recomprimir imagem não ajudaria neles:

```python
import re, collections, glob

for path in glob.glob('/mnt/user/Storage/appsdata/newpdfding/files/pdf/*.pdf'):
    d = open(path, 'rb').read()
    if len(d) < 15_000_000:  # abaixo de ~15 MB não vale o esforço de investigar
        continue
    kinds = collections.Counter()
    for m in re.finditer(rb'(\d+)\s+0\s+obj(.{0,600}?)stream\r?\n', d, re.S):
        hdr, start = m.group(2), m.end()
        e = d.find(b'endstream', start)
        if e < 0:
            continue
        ln = e - start
        for t in (b'JPXDecode', b'DCTDecode', b'JBIG2Decode', b'CCITTFaxDecode'):
            if t in hdr:
                kinds['imagem'] += ln
                break
        else:
            if b'FlateDecode' in hdr:
                kinds['vetor/conteudo'] += ln
    total = sum(kinds.values()) or 1
    frac_vetor = kinds['vetor/conteudo'] / total
    if frac_vetor > 0.7:  # mais de 70% do peso é conteúdo vetorial, não imagem
        print(f'{path}: {len(d)/1048576:.1f} MB, {frac_vetor:.0%} vetor — candidato a achatamento')
```

Um arquivo só entra na lista de "recomprimir imagens" (a outra recomendação da Fase 4, para os casos B e C do relatório 11) se a fração de `imagem` for dominante; senão, é candidato a achatamento como este documento descreve.

---

## 7. Resumo

- Achatar = trocar conteúdo vetorial por uma imagem renderizada da página. Serve para páginas com desenho vetorial pesado (mapas, diagramas complexos), não para escaneados.
- O maior arquivo do acervo é exatamente esse caso: 941 mil operações de path numa única página.
- 200–250 DPI é o ponto de partida para cartografia; abaixo disso, rótulos de mapa ficam ilegíveis.
- A busca da aplicação não é afetada — ela usa o texto já extraído e gravado em `pdf_text`, não o texto do PDF.
- Preservar a contagem de páginas e o tamanho físico de cada página é obrigatório para não quebrar anotações já salvas; `img2pdf` cuida disso automaticamente ao remontar a partir dos PNGs do Ghostscript.
- Reenvio via `PUT /api/pdfs/{id}/file`, já existente no backend, sem UI dedicada ainda.
- Nada foi executado contra o acervo real — este é um guia para execução manual, sob decisão do usuário sobre qual conteúdo vale a pena achatar.

---

**Arquivo deste relatório:** `refatoracao/12-achatamento-cartografia-vetorial.md`
