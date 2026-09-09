# Tickets — Physical Simulation V1

**Spec:** [`docs/spec/physical-simulation-v1.md`](../../spec/physical-simulation-v1.md)
**Modelo:** [`docs/architecture/physical-model.md`](../../architecture/physical-model.md)
**Decisão de origem:** [ADR 0006](../../adr/0006-physical-domain-model.md)

Nove tickets. O primeiro fecha decisões, os oito seguintes são fatias verticais: cada um termina com
um comando executável que produz um event log inspecionável, e nenhum deles entrega uma camada sem
um comportamento que a exercite.

A spec sugere na §15 uma ordem por módulo (`rng → types → dynamics → …`). Esta divisão **não** a
segue literalmente, porque uma ordem por módulo produz sete tickets sem nada observável e um oitavo
com todo o risco. A ordem abaixo preserva as mesmas dependências técnicas e agrupa os módulos pela
primeira pergunta que cada grupo responde.

---

## Tickets

| ID | Título | Pergunta que responde | Bloqueado por |
|---|---|---|---|
| [PSV1-0](00-decisoes-abertas.md) | Fechar as decisões abertas da §13 | Que forma tem `w_hab`, que regiões tem a fadiga, o que fazer sob ESS baixo, e Python vale para onde? | — |
| [PSV1-1](01-coorte-semeada.md) | Coorte semeada e reprodutível | Como é o corpo de 40 alunos sobre os quais o cânone nada diz? | PSV1-0 |
| [PSV1-2](02-corpo-que-persiste.md) | Corpo que persiste no tempo | O que três dias de sono ruim fazem com um corpo? | PSV1-1 |
| [PSV1-3](03-estimador-verdade-sintetica.md) | Estimador contra verdade sintética | Quanto um feat realmente restringe uma capacidade? | PSV1-1 |
| [PSV1-4](04-esforco-e-ocultacao.md) | Esforço resolvido e ocultação viável | Dá para terminar no meio do pelotão de propósito, e a que preço? | PSV1-2 |
| [PSV1-5](05-observacao-e-crenca.md) | Observação por observador e suspeita acumulada | O que cada um que assistiu viu, e o que passou a suspeitar? | PSV1-4 |
| [PSV1-6](06-interocepcao-e-fronteira.md) | Interocepção e fronteira de conhecimento | O que o próprio corpo informa ao dono dele — e o que nunca informa? | PSV1-5 |
| [PSV1-7](07-resolvedor-de-disputa.md) | Resolvedor de disputa por trocas | Quem controlou quem, quanto custou, e o que a luta revelou? | PSV1-5 |
| [PSV1-8](08-replay-e-snapshot.md) | Replay byte-a-byte e snapshot completo | O run é reexecutável, e o corpo atravessa o save? | PSV1-6, PSV1-7 |

## Grafo de dependência

```
PSV1-0 ── decisões (13.1 → PSV1-7, 13.2 → PSV1-2, 13.3 → PSV1-3, 13.4/13.5 → todos)
   │
   └─► PSV1-1 ─┬─► PSV1-2 ──► PSV1-4 ──► PSV1-5 ─┬─► PSV1-6 ─┐
               │                                  │           ├─► PSV1-8
               └─► PSV1-3 ─────────────────────────┴─► PSV1-7 ─┘
```

Paralelizável: **PSV1-2 e PSV1-3** depois do PSV1-1; **PSV1-6 e PSV1-7** depois do PSV1-5.
Caminho crítico: `0 → 1 → 2 → 4 → 5 → 7 → 8`.

## Convenções que valem para todos

1. **`src/embodiment/types.py` cresce por ticket.** Nenhum ticket introduz um tipo que não usa. Um
   `BodyState` completo no PSV1-1, antes de existir dinâmica, seria exatamente a camada sem
   comportamento que esta divisão evita.
2. **Cada ticket acrescenta um subcomando ao mesmo CLI** (`python -m embodiment …`). O CLI é a
   espinha do tracer bullet, não um extra.
3. **Cada ticket que introduz um arquivo em `data/models/physical/` passa pelo validador de cabeçalho
   entregue no PSV1-1**: `not_canon`, versão, proveniência, `evidence_sufficiency` próprio, e
   marcação `[INT]` do que é suposição. Parâmetro `[INT]` não declarado reprova o ticket.
4. **Nenhum ticket commita `capacity_posterior` de personagem canônico** nem compila perfil nominal
   para o engine (spec §12). A V1 inteira roda sobre coortes sintéticas e NPCs anônimos.
5. **Evidência de conclusão é o event log**, não a saída do terminal. Um ticket que só demonstre no
   stdout algo que o log não registra está incompleto.

## Cobertura da spec

Nenhum item da spec fica sem dono. Onde uma linha aparece em dois tickets, o **primeiro** é quem
implementa e o segundo é quem exercita em integração.

### Critérios de aceitação (spec §14)

| # | Critério | Ticket |
|---|---|---|
| 1 | Seeding com estrutura de correlação | PSV1-1 |
| 2 | Estimador, ESS, `evidence_sufficiency`, recuperação sintética | PSV1-3 |
| 3 | Nove canais dinâmicos com escalas de tempo certas | PSV1-2 (P7 no PSV1-6) |
| 4 | Dez passos do esforço, rejeição, três vereditos | PSV1-4 |
| 5 | Disputa por trocas, duas propriedades emergentes medidas | PSV1-7 |
| 6 | Observação por observador, log-odds perto do acaso | PSV1-5 |
| 7 | Fronteira de conhecimento, cinco classes de vazamento | PSV1-6 |
| 8 | Mesmo seed → mesmo log; metadados obrigatórios | PSV1-1 (seeding) → PSV1-8 (run completo) |
| 9 | Oito cenários do modelo §14 | distribuídos, ver abaixo |
| 10 | Honestidade dos parâmetros | validador no PSV1-1; um arquivo por ticket |
| 11 | Nenhum posterior canônico commitado | PSV1-3 (guarda) + PSV1-1 (lint de repositório) |
| 12 | Sem cliente de LLM, sem RNG global | PSV1-1 (testes de arquitetura) |

### Cenários de estresse (modelo §14)

Cada cenário vai para o ticket mais cedo em que a **asserção inteira** é verificável.

| Cenário | Ticket |
|---|---|
| 1 — meio do pelotão de propósito | PSV1-5 (observação idêntica) + PSV1-3 (posterior não separa) |
| 2 — desiste por tédio | PSV1-3 |
| 3 — semana de provas | PSV1-2 |
| 4 — lesão ocultada | PSV1-6 |
| 5 — sobrevivência na ilha | PSV1-4 |
| 6 — feat pós-divergência | PSV1-3 (informa seeding) + PSV1-6 (ausente de crença) |
| 7 — NPC sem evidência, criado tardiamente | PSV1-1 |
| 8 — testemunho | PSV1-3 |

### Property tests (spec §11.2)

| ID | Propriedade | Ticket |
|---|---|---|
| P1 | cena não cura | PSV1-2 |
| P2 | monotonicidade de custo | PSV1-4 |
| P3 | validade de estado | PSV1-2 |
| P4 | piso nunca vira teto | PSV1-3 |
| P5 | isolamento de substream | PSV1-1 (seeding) → PSV1-8 (run completo) |
| P6 | determinismo | PSV1-1 (seeding) → PSV1-8 (run completo) |
| P7 | sem telemetria | PSV1-6 |

### Modos de falha (spec §8)

| Falha | Ticket | Falha | Ticket |
|---|---|---|---|
| F1 fusão capacidade/desempenho | PSV1-1, PSV1-4 | F9 teto sem atestação | PSV1-3 |
| F2 cena curando o corpo | PSV1-2 | F10 detecção saturando | PSV1-5 |
| F3 atributo à mão | PSV1-1 | F11 DOMS imediato | PSV1-2 |
| F4 comparação como entrada | PSV1-1 | F12 `impairment` global | PSV1-2, PSV1-4 |
| F5 re-sorteio de capacidade | PSV1-1 | F13 LLM narrando estado | PSV1-4 |
| F6 ordem de criação | PSV1-1 | F14 razão aguda:crônica | PSV1-4 |
| F7 marginais independentes | PSV1-1 | F15 estado corporal inválido | PSV1-2 |
| F8 degeneração de partículas | PSV1-3 | F16 penalidade térmica por idade | PSV1-2, PSV1-4 |

### Invariantes do modelo (§15) e do `CONTEXT.md`

| Invariante do modelo | Ticket |
|---|---|
| 1 seis estruturas distintas | PSV1-1, PSV1-4 |
| 2 desempenho restringe como limite | PSV1-3 |
| 3 ausência de evidência → prior | PSV1-1 |
| 4 `BodyState` persiste | PSV1-2 |
| 5 só o engine escreve estado físico | PSV1-2 (API única), PSV1-4 |
| 6 interocepção qualitativa | PSV1-6 |
| 7 ocultação como intenção de primeira classe | PSV1-4, PSV1-5 |
| 8 comparações são saída | PSV1-1 |
| 9 unidade física, percentil derivado | PSV1-1 |
| 10 capacidade × habilidade × disposição | PSV1-4, PSV1-7 |
| 11 substreams nomeados e metadados de run | PSV1-1, PSV1-8 |
| 12 feat pós-divergência | PSV1-3, PSV1-6 |
| 13 resolução física é subdomínio do engine | PSV1-1 |

Os testes nomeados `test_invariant_<n>_<slug>` da spec §11.4 vivem em `tests/embodiment/invariants/`,
um arquivo por invariante, acrescentados pelo ticket dono de cada linha acima.

### Evals versionadas vazias (spec §11.6)

| Família | Ticket que cria o esqueleto |
|---|---|
| `knowledge-boundary` | PSV1-6 |
| `simulation-evals` | PSV1-7 |

Ambas ficam versionadas e **vazias de comportamento dependente de modelo** na V1, porque a V1 não tem
nenhum. Elas existem para bloquear a integração seguinte, não esta.

## Fora do escopo de todos os tickets

Repetido aqui para que nenhum ticket precise ser lido por inteiro para descobrir o que não fazer
(spec §3.2): deriva de `capacity_baseline` ao longo do ano; ingestão de feats canônicos reais;
posteriores por personagem commitados; integração com `ExamSpec`; `ExertionIntent` escolhido por LLM;
UI; dinâmica de doenças; reputação de força.
