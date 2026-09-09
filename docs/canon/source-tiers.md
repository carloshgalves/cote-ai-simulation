# Camadas de fonte

## Princípio

**Tier é função de autoria e forma, não de canal de distribuição.**

O Volume 0 foi distribuído como bônus de BD/DVD, mas é prosa de light novel escrita pelo autor original, com prólogo, capítulos e epílogo. Ele ocupa o mesmo tier dos demais volumes da mesma edição. Uma ficha de personagem impressa dentro de um volume da light novel, por outro lado, é paratexto: material complementar oficial, não prosa narrativa, e vive em tier próprio.

## Escala

| Tier | Fonte | Uso |
|---|---|---|
| **0** | Prosa de light novel, edição original em japonês — volumes principais, volumes `.5` e Volume 0 | autoridade máxima |
| **1** | Prosa de light novel, tradução oficial licenciada (Seven Seas EN) | autoridade de trabalho |
| **2** | Material complementar oficial do autor/editora: fichas do School Database, posfácios, perfis de site oficial, drama CDs, encartes, publicações oficiais | primária para atributos de entidade; **não** para regras |
| **3** | Mangá (adaptação licenciada) | secundária |
| **4** | Anime (adaptação; diverge do texto) | apenas `continuity: anime`; nunca semeia world truth |
| **5** | Wiki, fandom, traduções e sumários de comunidade | **descoberta e localização apenas** |
| **6** | Memória do modelo de linguagem | **nunca é fonte**; força `UNVERIFIED` |

## Portões

| Para atingir | Exigência |
|---|---|
| `epistemic_status: VERIFIED` em `INSTITUTIONAL_RULE` ou `WORLD_TRUTH` | ≥1 `supports` em Tier 0–1, com `verified_by` e `verified_at` |
| `epistemic_status: VERIFIED` em `ENTITY` | ≥1 `supports` em Tier 0–2 |
| compilar para `data/canon/derived/` (consumo pelo engine) | `VERIFIED` + `engine.binds_to` preenchido + nenhum `conflict` aberto |
| `strength: primary` numa referência | Tier 0–3 |

Tier 4 e 5 nunca aparecem em `supports`. Aparecem em `discovered_via`.

## Divergências conhecidas entre mídias

O anime diverge do texto em pontos verificáveis (por exemplo, tamanho das turmas). Claims de origem exclusivamente adaptacional recebem `continuity: [anime]` e ficam fora da recuperação padrão, que opera em `continuity: [ln]`.
