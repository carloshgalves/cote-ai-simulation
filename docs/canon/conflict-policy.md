# Política de conflitos entre fontes

## Regras

1. **Cross-tier** — o tier mais alto prevalece. O lado perdedor é preservado como `variant` com sua própria `continuity`; nunca é apagado.
2. **Mesmo tier** — nenhum lado vence automaticamente. Abre-se `conflict` e `open_question`; os claims envolvidos permanecem `UNVERIFIED` e não compilam.
3. **Retcon intra-light-novel** — volume posterior contradiz volume anterior sobre o mesmo intervalo de validade: o posterior prevalece, ligados por `supersedes` / `superseded_by`, ambos preservados.
4. **Regra declarada × comportamento observado** — ver abaixo.
5. **Aritmética de terceiros** — sempre `INFERRED`, nunca `VERIFIED`, até recomputação a partir de claims verificados.
6. **Conteúdo exclusivo de adaptação** — `continuity: [anime]` ou `[manga]`, fora da recuperação padrão.

## Regra 4 em detalhe

Quando uma regra declarada no texto e o comportamento observado do mundo divergem, **o comportamento observado não invalida automaticamente a regra declarada**. Ambos são registrados como claims distintos e um `conflict` é aberto com o conjunto de hipóteses examinado explicitamente:

| Hipótese | Significado |
|---|---|
| `exception` | a regra vale, e o caso observado é uma exceção prevista |
| `narrower_scope` | a regra vale, mas o escopo real é mais estreito que o enunciado |
| `temporal_change` | ambos estão corretos em intervalos de validade diferentes |
| `unreliable_declaration` | a fonte da declaração é fraca (adaptação, wiki, personagem não confiável) |
| `unreliable_observation` | a observação é fraca ou mal reconstruída |
| `misreading` | nossa leitura de um dos lados está errada |

Enquanto o conflito estiver aberto, **nenhum dos dois lados compila** para o engine. Não arbitramos por conveniência de implementação.

Quando uma escolha de modelagem funciona sob qualquer resolução do conflito, ela pode ser adotada — mas é registrada como decisão de modelagem, não como resolução do conflito.

## Declarante não confiável

Regras enunciadas por personagens dentro da obra podem ser incompletas, interessadas ou falsas. Uma regra enunciada por um personagem é registrada como `claim_kind: BELIEF` com `holder`, e só vira `INSTITUTIONAL_RULE` quando o texto ou o comportamento do mundo a confirmam de forma independente.

## Registro

Cada conflito vive em `data/canon/conflicts/<id>.yaml` e é referenciado por `provenance.conflicts[]` em todos os claims envolvidos, nos dois sentidos.
