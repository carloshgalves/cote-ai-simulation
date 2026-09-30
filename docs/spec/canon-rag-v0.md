# Canon RAG V0 — gated, provider-neutral retrieval

**Status:** Accepted  
**Data:** 2026-09-29  
**Origem:** ADR-0001, ADR-0002 e ADR-0005  
**Auditoria:** [rag-v0-audit.md](../canon/rag-v0-audit.md)

## 1. Problema e objetivo visível

O Canon Knowledge Base já separa verdade, conhecimento, tempo e proveniência, mas ainda não
possui uma superfície de recuperação capaz de provar que contexto proibido fica de fora antes
de ranking/context assembly.

A V0 deve demonstrar deterministicamente:

> mesma pergunta + personagens diferentes + mesmo instante pode produzir evidências diferentes
> quando o conhecimento ou o sujeito da evidência difere.

e:

> uma evidência proibida nunca chega ao contexto, ainda que fosse a melhor correspondência lexical
> ou semântica.

A V0 é provider-neutral e rebuildable. Não escolhe vector DB, embeddings, framework de agentes
nem linguagem do runtime de produção.

## 2. Invariantes

1. Claims/evidence estruturados são autoridade. Retrieval records, chunks, scores, índices e
   embeddings são artefatos derivados e descartáveis.
2. RAG nunca escreve world truth.
3. Gate de autorização ocorre **antes** do ranking.
4. Metadado necessário ausente ou incomparável = exclusão, nunca permissividade.
5. `UNVERIFIED`, `INTERPRETATION` e hipótese não entram em lane factual.
6. Evidência de outro personagem não é Character Core nem memória do ator consultado.
7. O retriever nunca usa `known_by` canônico para “atualizar” conhecimento depois de
   `Y1_START`.
8. Context assembly mantém separadas as camadas do ADR-0002.
9. Nenhum texto integral protegido é persistido no repositório público.
10. Um índice incapaz de aplicar os gates antes de devolver candidatos não satisfaz este contrato.

## 3. Escopo

### Dentro

- CanonEvidence V2;
- compilação conceitual `CanonEvidence + CanonClaim + SourceRegistry → RetrievalRecord`;
- corpus manifest;
- request/result provider-neutral;
- quatro gates do ADR-0005 + gate epistêmico/conflito;
- lexical/reference retriever determinístico apenas como oracle de conformidade;
- context assembly de duas lanes canônicas;
- suíte de golden retrieval evals;
- corpus vertical pequeno dos seis personagens focais.

### Fora

- Episodic Memory;
- Current Beliefs pós-seed;
- reflexões/summaries;
- goals/plans;
- Character Cognition;
- event-log/CSF integration;
- vector DB ou embeddings definitivos;
- fine-tuning;
- corpus integral da light novel;
- resolução das 28 open questions;
- promoção de qualquer record atual a VERIFIED sem leitura humana.

## 4. Termos

### 4.1 CanonEvidence

Record autorado e versionado, contendo **nossa paráfrase** de uma passagem. Não é uma segunda
fonte de verdade. Seu texto pode apoiar recuperação, mas fatos continuam apontando para claims.

Cada evidence V2 possui:

- `id` / evidence_id;
- `retrieval_role`:
  - `BEHAVIORAL_CANON`: material para caracterização/comportamento;
  - `KNOWLEDGE_EVIDENCE`: material que pode sustentar contexto factual apenas quando o claim
    correspondente já estiver autorizado no knowledge scope;
- `text`;
- `story_time`;
- `narrative_position: {work, ordinal}`;
- `actors[]`;
- `supports_claims[]`;
- `topics[]`;
- `situation_tags[]`;
- `provenance`.

A V0 exige exatamente uma referência `strength: primary` por evidence. Referências
`corroborating` podem existir, mas não mudam a identidade narrativa da unidade.

### 4.2 RetrievalRecord

Artefato derivado, nunca editado à mão. Uma unidade recuperável mantém identidade 1:1 com um
CanonEvidence. Campos mínimos:

- `record_id`: derivado de `evidence_id`;
- `evidence_id`;
- `source_id`: hash determinístico de `work + edition + locator` da referência primária;
- `work`;
- `locator`;
- `narrative_ordinal`;
- `actors[]`;
- `supported_claim_ids[]`;
- `effective_time`;
- `retrieval_role`;
- `epistemic_status`;
- `open_conflicts[]`;
- `topics[]` / `situation_tags[]`;
- `text`;
- `provenance` completa o suficiente para reabrir o record fonte;
- `source_digest` para detectar corpus stale.

Campos de visibility/knowledge são **compilados como autorização**, não copiados como verdade
independente. O result registra quais claim IDs do request autorizaram a passagem.

### 4.3 CorpusManifest

Artefato derivado contendo:

- schema/manifest version;
- digest dos arquivos fonte;
- lista de evidence IDs;
- work IDs e versões;
- algoritmo de compilação;
- algoritmo de chunking;
- backend id;
- timestamp apenas informativo.

O mesmo conjunto de fontes + mesma versão de compiler produz o mesmo digest e os mesmos records.

## 5. Chunking V0

A unidade mínima e padrão é **um CanonEvidence**.

Regras:

1. máximo de **1600 Unicode code points** no `text` público;
2. zero overlap na V0;
3. evidence maior é rejeitado pelo compiler e deve ser reescrito em unidades canônicas menores;
4. nunca agrupar evidence de obras ou cenas primárias distintas em um único retrieval record;
5. chunks de corpus privado, se um adapter futuro os produzir, devem manter
   `parent_evidence_id`, source/provenance e todos os metadados de gate;
6. chunk privado nunca é commitado;
7. ranking pode colapsar múltiplos chunks privados de volta ao `evidence_id`, mas não pode apagar
   a identidade do parent;
8. rebuild completo deve ser sempre possível somente a partir dos records autoritativos e do
   corpus privado legitimamente disponível.

O limite é por code points, não tokens, para não acoplar o contrato a um tokenizer/provedor.

## 6. RetrievalRequest

Forma lógica provider-neutral:

```yaml
character_id: actor...
simulation_time: <opaque canonical/engine time ref>
divergence_time: <time ref|null>
knowledge_horizon:
  <work-id>: <max narrative ordinal>
query:
  text: ...
  purpose: BEHAVIORAL_GUIDANCE | KNOWN_FACTS
knowledge_scope:
  mode: CANON_SEED | SIMULATION
  allowed_claim_ids: [...]
filters:
  retrieval_roles: [...]
  topics: [...]
  evidence_ids: [...]
top_k: 5
allow_unverified_behavioral: false
```

Regras:

- `CANON_SEED` só é válido no instante de seed definido pelo caller, normalmente
  `Y1_START`.
- em `SIMULATION`, `allowed_claim_ids` vem do futuro belief/event-log adapter. Se estiver
  ausente, `KNOWN_FACTS` abstém/falha fechado.
- o retriever não deriva conhecimento pós-seed de `known_by` canônico.
- `knowledge_horizon` é allow-list por obra. Obra ausente está fora do horizonte.
- filtros ausentes ou com lista vazia não restringem candidatos; listas não vazias de
  `retrieval_roles`, `topics` e `evidence_ids` são allow-lists aplicadas antes dos gates.
- `top_k >= 1`.

## 7. Pipeline e gates

Pipeline obrigatório:

```
compile authoritative records
  → deterministic prefilters
  → effective_at
  → actor_gate
  → spoiler_horizon
  → divergence_gate
  → epistemic/conflict gate
  → rank only eligible candidates
  → top-K
  → context assembly
```

### 7.1 effective_at(t_sim)

O adapter temporal deve ser capaz de provar que o evidence/claim é aplicável até
`simulation_time`. Se a relação temporal não puder ser comparada, exclui.

Para `KNOWLEDGE_EVIDENCE`, todos os claims usados para autorização precisam ser temporalmente
compatíveis. Para `BEHAVIORAL_CANON`, o evento exemplificador precisa ter ocorrido até
`simulation_time` na V0; usar feats comportamentais futuros antes de ocorrerem fica fora desta
versão conservadora.

### 7.2 actor_gate(actor, t_sim)

- `BEHAVIORAL_CANON`: `character_id` precisa constar em `actors[]`.
- `KNOWN_FACTS`: pelo menos um `supported_claim_id` precisa constar em
  `knowledge_scope.allowed_claim_ids`.
- grupos (`group.first-years`) não são expandidos por palpite. Um futuro seed builder resolve
  membership e produz os claim IDs autorizados.

### 7.3 spoiler_horizon(work, locator)

A comparação usa `narrative_position.ordinal` e o cap por work do request.

- work ausente → exclui;
- ordinal ausente → exclui;
- ordinal acima do cap → exclui.

O ordinal é uma chave de ordenação do corpus, não uma afirmação de story time.

### 7.4 divergence_gate(t_div)

Se não há divergência, passa.

Se evidence ocorre depois da divergência:

- `KNOWLEDGE_EVIDENCE` → **proibido**;
- `BEHAVIORAL_CANON` → pode sobreviver apenas como lane comportamental e nunca como memória,
  world truth atual ou claim aprendido na simulação.

Essa exceção não autoriza spoiler além de `knowledge_horizon`.

### 7.5 Epistemic/conflict gate

`KNOWN_FACTS` V0 aceita apenas claims/evidence `VERIFIED`, sem conflito aberto relevante.

`BEHAVIORAL_GUIDANCE`:
- `VERIFIED` passa;
- `UNVERIFIED` só passa com `allow_unverified_behavioral: true`;
- `INFERRED`/`INTERPRETATION` só poderão ser liberados por versão futura; V0 abstém.

Opt-in de `UNVERIFIED` é para pesquisa/ensaio de arquitetura, nunca promoção a verdade.

## 8. Ranking V0

O backend de referência é lexical e determinístico:

- normalização Unicode + casefold;
- tokenização simples por palavras;
- score por overlap normalizado;
- tie-break por `evidence_id`.

Ele existe para provar interface/gates, não como decisão de qualidade de busca.

Um backend futuro pode usar embeddings/reranking se produzir o mesmo conjunto de candidatos
**elegíveis** antes de ordenar.

## 9. RetrievalResult

Cada item inclui:

- evidence_id;
- score e componentes do score;
- work, locator e source_id;
- supported claim IDs;
- retrieval role;
- epistemic status;
- gate trace;
- authorization claim IDs;
- provenance;
- corpus manifest digest.

Resultados inelegíveis não entram em `items`. Em modo de eval/debug, podem aparecer apenas em
`excluded[]` com reason code, nunca no payload destinado ao agente.

Reason codes V0:
`NOT_EFFECTIVE`, `WRONG_ACTOR`, `CLAIM_NOT_ALLOWED`, `SPOILER`,
`POST_DIVERGENCE_FACT`, `UNVERIFIED_FACT`, `UNVERIFIED_BEHAVIOR_DISABLED`,
`NON_FACTUAL_STATUS`, `OPEN_CONFLICT`, `MISSING_GATE_METADATA`,
`FILTERED_OUT`.

## 10. Context assembly

O assembler produz somente duas lanes canônicas:

```
CANON_BEHAVIORAL_EVIDENCE
  - examples about how this character has behaved
  - never facts about current simulated events

KNOWN_CANON_FACTS
  - only claims already authorized by knowledge_scope
  - never secret world truth
```

Ele não cria nem mistura:

- Character Core;
- Static Background Knowledge;
- Current Beliefs;
- Episodic Memory;
- Derived Reflections/Summaries;
- Current Goals/Plans.

Essas camadas podem ser entradas separadas de um futuro context builder, mas a RAG V0 não as
sintetiza.

## 11. Evals

Cada golden query registra:

- id;
- character;
- simulation_time/divergence_time;
- horizon;
- query/purpose;
- allowed claims;
- expected evidence IDs;
- acceptable evidence IDs;
- forbidden evidence IDs com reason esperado;
- abstention permitido/obrigatório.

Cobertura mínima:

1. recuperação normal;
2. spoiler futuro;
3. segredo de outro personagem;
4. cânone pós-divergência;
5. source/claim conflitante;
6. evidence insuficiente;
7. mesma pergunta, dois personagens, resultados diferentes;
8. nenhum evidence elegível.

Métricas/checks:

- hit@K / recall@K;
- reciprocal-rank como ranking quality V0;
- irrelevant-context rate;
- wrong-character leakage;
- forbidden-timeline leakage;
- post-divergence canon leakage;
- provenance completeness.

Qualquer leakage proibido é falha dura, independentemente das médias.

## 12. Observabilidade e reprodutibilidade

Uma execução de eval registra:

- manifest digest;
- golden suite version;
- retriever backend id/version;
- request normalizado;
- result evidence IDs;
- exclusion reason codes;
- métricas agregadas.

Nenhum score de LLM é necessário na V0.

## 13. Migração

`data/canon/evidence/` está vazio no main auditado. Portanto CanonEvidence V1 → V2 é uma mudança
de schema sem migração de records existentes.

Nenhum snapshot persistido de RAG existe.

## 14. Decisões abertas

Não escondidas em critérios de aceitação:

- stack/language do runtime de produção;
- vector DB;
- embeddings;
- semantic reranker;
- formato do adapter de tempo definitivo da CSF;
- actor/group registry;
- private-corpus adapter.

Nenhuma bloqueia o reference conformance slice.

## 15. Critérios de aceitação V0

1. seis personagens focais possuem ao menos uma evidence paraphrase UNVERIFIED, rastreável e
   explicitamente não factual.
2. schema V2 impede evidence sem papel e posição narrativa.
3. corpus manifest é determinístico/rebuildable.
4. um oracle lexical executa gates antes do ranking.
5. golden suite cobre os oito cenários mínimos.
6. mesmo query/time com personagens diferentes pode produzir evidence sets diferentes.
7. um candidato semanticamente/lexicamente melhor mas proibido não chega a context.
8. factual request sobre corpus apenas UNVERIFIED abstém.
9. nenhum arquivo protegido integral é commitado.
10. nenhum módulo de produção escolhe Python, vector DB ou provedor.
