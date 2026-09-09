# Proveniência

## Três campos, não um

A exigência "wiki para descobrir, light novel para afirmar" só é executável se descoberta e sustentação forem campos diferentes.

| Campo | Pergunta | Quem pode aparecer |
|---|---|---|
| `supports[]` | o que **sustenta** a afirmação? | Tier 0–3 |
| `discovered_via[]` | como **localizamos** a afirmação? | qualquer tier, tipicamente 4–6 |
| `verified_by` / `verified_at` | quem conferiu no texto, e quando? | humano |

Uma referência nunca migra de `discovered_via` para `supports`. Ler o wiki não converte o wiki em light novel; converte o wiki em ponteiro para o volume e o capítulo onde a leitura deve acontecer.

```yaml
provenance:
  epistemic_status: UNVERIFIED
  supports:
    - work: ln.y1.v01
      locator: {chapter: 1, scene: "homeroom / distribuição dos cartões"}
      edition: seven-seas-en
      quote_policy: paraphrase-only
      strength: primary
  discovered_via:
    - tier: 5
      ref: "you-zitsu.fandom.com/wiki/Advanced_Nurturing_High_School"
      retrieved_at: 2026-09-08
  verified_by: null
  verified_at: null
  continuity: [ln]
  conflicts: []
```

## Enquanto `verified_by` for nulo

`epistemic_status` fica travado em `UNVERIFIED` ou `INFERRED`. Não há caminho para `VERIFIED` sem um humano registrando que abriu o volume.

## Política de citação

`quote_policy: paraphrase-only` é o padrão e reflete a política de `data/canon/README.md` e de `AGENTS.md`: o repositório público não armazena texto integral protegido. O `locator` existe para rastreabilidade; a paráfrase existe para uso. Corpus original fica em `data/canon/private/` ou `raw/`, ignorados pelo Git.

## Derivações

Claims com `epistemic_status: INFERRED` registram como foram obtidos:

```yaml
derivation:
  method: arithmetic          # arithmetic | logical | corroboration
  from: [claim.a, claim.b]
  computation: "1000 - 1000 = 0"
  recomputed_by: null
```

Aritmética de terceiros (tabelas de progressão de wiki, somatórios de fãs) nunca entra como `VERIFIED`. Entra como `INFERRED` com `recomputed_by` nulo até refazermos a conta a partir de claims verificados.
