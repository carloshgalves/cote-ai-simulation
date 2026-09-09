# Taxonomia

Três eixos independentes. Um claim tem posição em cada um dos três, simultaneamente.

## Eixo 1 — `claim_kind`: o que a proposição é, no universo

| Valor | Significado |
|---|---|
| `WORLD_TRUTH` | fato objetivo do universo, independente de quem o conhece |
| `INSTITUTIONAL_RULE` | regra objetiva da escola (subtipo de world truth com semântica operacional) |
| `EVENT` | ocorrência datada na linha canônica |
| `ENTITY` | existência e atributos de um objeto do domínio (pessoa, classe, instalação, órgão) |
| `BELIEF` | estado mental de um ator; **pode ser falso** |

## Eixo 2 — `visibility` e `known_by`: quem pode saber, no universo

`visibility_default` descreve a situação no limite superior de `effective_from`:

| Valor | Significado |
|---|---|
| `PUBLIC` | normalmente disponível a qualquer ator do escopo |
| `RESTRICTED` | disponível a um grupo definido (faculdade, conselho estudantil, uma classe) |
| `PRIVATE` | disponível a atores nomeados |
| `CONCEALED` | verdadeiro, mas nenhum ator do escopo conhece |

`visibility` **não é escalar**: é uma função do tempo. O que é público em maio não era público em abril. O campo escalar é apenas o padrão inicial; a verdade operacional está em `known_by[]` e `unknown_by[]`.

`unknown_by[]` (asserção negativa explícita) tem o mesmo peso de `known_by[]`. É o que permite auditar vazamento em vez de apenas confiar em omissão.

## Eixo 3 — `epistemic_status`: nossa confiança como pesquisadores

| Valor | Critério |
|---|---|
| `VERIFIED` | conferido diretamente em fonte Tier 0–1 (ver [`source-tiers.md`](source-tiers.md)), com `verified_by` e `verified_at` preenchidos |
| `INFERRED` | conclusão nossa, derivada de claims verificados, com derivação registrada |
| `INTERPRETATION` | leitura plausível não confirmada pelo texto |
| `UNVERIFIED` | ainda sem sustentação suficiente; inclui tudo apoiado apenas por Tier 4–5 |

## Mapeamento dos rótulos originais

| Rótulo | Representação |
|---|---|
| WORLD_TRUTH | `claim_kind: WORLD_TRUTH` |
| INSTITUTIONAL_RULE | `claim_kind: INSTITUTIONAL_RULE` |
| PUBLIC_KNOWLEDGE | `visibility: PUBLIC` |
| PRIVATE_KNOWLEDGE | `visibility: PRIVATE` ou `RESTRICTED` |
| BELIEF | `claim_kind: BELIEF` + `holder` + `truth_value` |
| INFERENCE (nossa) | `epistemic_status: INFERRED` |
| INFERENCE (do personagem) | `claim_kind: BELIEF` + `derivation: inference` |
| INTERPRETATION | `epistemic_status: INTERPRETATION` |
| UNVERIFIED | `epistemic_status: UNVERIFIED` |

A ambiguidade de "INFERENCE" é real e é resolvida na origem: quem inferiu, nós ou o personagem, são fatos de natureza completamente diferente e vivem em eixos diferentes.
