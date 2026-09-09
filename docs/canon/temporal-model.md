# Modelo temporal

## Os quatro tempos

| Eixo | Pergunta | Onde vive |
|---|---|---|
| **Valid time** | desde quando é verdade no universo? | `temporal.effective_from` / `effective_to` |
| **Narrative time** | onde a obra revela ao leitor? | `provenance.supports[].work` + `locator` |
| **Knowledge time** | desde quando cada ator sabe? | `known_by[].since` / `unknown_by[].until` |
| **Transaction time** | desde quando *nós* afirmamos isso? | commit Git + `revision` |

Confundir narrative time com valid time é o erro central que este modelo existe para impedir. Uma regra explicada no Volume 7 quase sempre já valia em abril do primeiro ano.

## `TimeBound`: origem desconhecida é um valor legítimo

Observar que uma regra operava em T prova exatamente uma coisa: **ela existia em T ou antes**. Não prova que existia desde a fundação da escola, nem desde qualquer outra âncora conveniente.

```yaml
effective_from:
  mode: HOLDS_BY            # EXACT | HOLDS_BY | INTERVAL | UNKNOWN
  holds_by:                 # provado: já valia aqui
    anchor: Y1_START
  origin: UNKNOWN           # obrigatório quando mode != EXACT
  basis: >
    A dedução de class points do primeiro mês foi aplicada sobre
    comportamento de abril, logo a regra já operava no dia 1.
```

| `mode` | Quando usar | Campos |
|---|---|---|
| `EXACT` | o texto declara o momento da instituição | `exact` |
| `HOLDS_BY` | provado que já valia em um ponto; origem desconhecida | `holds_by`, `origin: UNKNOWN`, `basis` |
| `INTERVAL` | **ambos** os limites têm suporte textual | `not_before` + `not_before_support`, `holds_by` |
| `UNKNOWN` | nem o limite superior é conhecido | `basis` |

Regras duras:

- É proibido usar `SCHOOL_FOUNDING`, `PRE_Y1` ou qualquer âncora como origem presumida. `SCHOOL_FOUNDING` só aparece em `mode: EXACT`, quando o texto o afirma.
- `not_before` só existe acompanhado de `not_before_support` apontando para fonte. O schema recusa o contrário.
- `HOLDS_BY` é o caso majoritário e não é um defeito do registro. É a descrição correta do que sabemos.

### Por que isso não bloqueia a simulação

Para semear o mundo em `Y1_START` a pergunta é `holds_at(Y1_START)`, respondida por:

```
holds_at(t) := (mode == EXACT   and exact <= t)
            or (mode == HOLDS_BY and holds_by <= t)
            or (mode == INTERVAL and holds_by <= t)
```

O limite superior sozinho decide. A origem real nunca é necessária para simular o primeiro ano — e por isso nunca precisa ser inventada. Ela só importaria para simular a história da escola, que está fora de escopo.

## Procedimento de decisão para `effective_from`

```
A proposição descreve...
├─ um EVENTO datado?
│     → mode: EXACT, exact = data do evento
├─ uma REGRA / ESTRUTURA / PROPRIEDADE estável?
│   ├─ o texto declara quando foi instituída?
│   │     → mode: EXACT
│   ├─ o texto mostra a regra operando em algum momento?
│   │     → mode: HOLDS_BY, holds_by = primeira operação observada, origin: UNKNOWN
│   └─ só há a revelação, sem operação observada?
│         → mode: HOLDS_BY com holds_by = ponto da revelação, e abrir open_question
├─ uma MUDANÇA de regra?
│     → novo claim, mode: EXACT na data da mudança, supersedes = claim anterior
└─ um estado mental?
      → claim_kind: BELIEF, intervalo próprio, nunca WORLD_TRUTH
```

A pergunta a fazer em cada caso continua sendo: *isso passou a ser verdade naquele momento, ou apenas foi revelado ao leitor naquele momento?*

## Âncoras

Ano letivo japonês, abril a março, três semestres. Datas em `Y<n>-MM-DD` (ano relativo). A obra nunca fixa o ano civil; amarrar a um ano real seria inventar cânone.

```
WHITE_ROOM_FOUNDING · SCHOOL_FOUNDING · ADMISSION_DECISION · PRE_Y1
Y1_START · Y1_M01 … Y1_M12 · Y1_SEM1 · Y1_SEM2 · Y1_SEM3 · Y1_END
```

Âncoras de exame (`Y1_MIDTERM_1`, `Y1_ISLAND`, …) são declaradas em `data/canon/y1/events/`.

## Os quatro portões de recuperação

Toda leitura do KB por engine, context builder ou RAG passa por quatro filtros independentes:

1. **`effective_at(t_sim)`** — o fato vale agora no mundo?
2. **`actor_gate(actor, t_sim)`** — este ator pode saber?
3. **`spoiler_horizon(work, locator)`** — a evidência vem de um ponto da obra além do horizonte configurado?
4. **`divergence_gate(t_div)`** — evidência canônica posterior à divergência entra como evidência de *disposição comportamental*, nunca como memória factual.

### Fronteira do `known_by`

O `known_by` canônico é autoritativo **apenas em `t = Y1_START`**, para semear o belief state inicial. A partir daí quem governa é o event log da simulação.

Sem essa fronteira o KB vira um canal lateral: um personagem "saberia" em novembro algo que na linha simulada nunca aconteceu.
