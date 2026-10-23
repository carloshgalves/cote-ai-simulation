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

## Operador: edições autorizadas para packs de simulação (22-10-2026)

A autoridade bibliográfica de Tier 0–6 continua servindo como referência de origem e qualidade editorial. O **operador** pode aceitar explicitamente uma edição traduzida de fãs para fundamentar os packs internos, sem reclassificá-la falsamente como tradução oficial. Para essas edições, a aprovação de fonte é registrada separadamente da **auditoria por passagem** e não produz `VERIFIED` automático nem contorna as barreiras de conhecimento, cronologia e divergência. Consulte [operator-source-acceptance.md](operator-source-acceptance.md).

## Exceção explícita: edição aceita pelo operador

O operador pode autorizar a utilização direta de uma edição/tradução fornecida por ele como **fonte de trabalho primária para extração de packs**, inclusive tradução de fãs. A autorização deve conter `source_id`, edição, digest e data de aceite. Não declara que a tradução é oficial nem que as inferências do extrator estão corretas.

Para novas camadas de referência V2, registrar `operator_accepted: true` e `claims_all_verified: false` até a auditoria de cada afirmação. O aceite autoriza **leitura, citação por localizador e extração**, mas não remove os filtros de tempo, conhecimento, conflitos ou evidência de comportamento.

Os compiladores legados de claims que hoje exigem Tier 0–1 **continuam com seus portões intactos**. A elegibilidade de uma tradução aceita pelo operador para a compilação factual exige uma futura migração explícita e testada dos schemas e gates, sem contornar os testes atuais. Não reclassificar uma tradução de fãs como tradução oficial.
