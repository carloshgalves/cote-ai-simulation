# PSV1-1 — Coorte semeada e reprodutível

**Spec:** §3.1 (`POPULATION_PRIOR`, `WORLD_SEEDING`, `RNG`, `RUN_METADATA`) · §5.1 · §5.2 · §9 · §14.1
**Bloqueado por:** PSV1-0 (13.4)
**Bloqueia:** PSV1-2, PSV1-3

## Resultado observável

```
python -m embodiment seed-cohort --world-seed 42 --n 40 --out runs/demo/
```

semeia 40 alunos anônimos a partir do prior populacional, congela um `capacity_baseline` por aluno,
escreve `capacity.sampled` no event log e um snapshot com `run_metadata` completo. Rodar de novo com
`--world-seed 42` produz o mesmo log. Rodar com `--n 41` produz os mesmos 40 corpos de antes, mais
um.

A coorte resultante **não** contém o aluno simultaneamente mais pesado, mais rápido e melhor no
vaivém — a estrutura de correlação do prior aparece na amostra.

Este é o primeiro ticket porque é o menor caminho que já exercita a invariante que a V1 inteira
existe para provar: nenhum atributo físico escolhido à mão, em lugar nenhum, para ninguém.

## Escopo

- **Esqueleto executável.** `pyproject.toml`, pacote `src/embodiment/`, `tests/embodiment/`, `pytest`
  + `hypothesis` + `numpy` + `pydantic`. Sem framework de aplicação: a V1 não tem serviço.
- **`rng.py`.** `substream(world_seed, character_id, event_id, purpose) -> numpy.random.Generator`,
  via `SeedSequence.spawn` derivado do **nome** do substream. Nunca `numpy.random` global, nunca
  `random` da stdlib.
- **`types.py`, só o que este ticket usa.** `CapacityProfile` e o registro de amostra congelada, com
  unidade física por dimensão e as 15 dimensões do enum do modelo §4. Percentil é *view* derivada,
  jamais armazenamento.
- **`prior.py`.** Carrega `population-prior.yaml`; amostra por cópula gaussiana sobre as marginais,
  com dois fatores latentes (aptidão geral e porte) mais resíduo por dimensão.
- **`seeding.py`.** Único lugar do repositório autorizado a escrever `capacity_baseline`. Amostra uma
  vez, congela, grava. Personagem sem restrição alguma amostra do prior — que é o posterior de zero
  evidência, com `evidence_sufficiency = 0` por dimensão. O PSV1-3 entra por trás desta mesma
  assinatura, sem alterá-la.
- **Event log e `run_metadata`.** Append-only; `capacity.sampled` com `character_id`,
  `posterior_hash`, `prior_version`, substream e `evidence_sufficiency` por dimensão. O run **falha
  ao iniciar** se faltar qualquer metadado obrigatório da §9.2.
- **`snapshot.py`, primeira metade.** Serializa `characters.<id>.capacity_baseline` + `run_metadata`,
  com hash. A seção `body_state` entra no PSV1-2; `physical_beliefs` no PSV1-6.
- **Validador de cabeçalho de `data/models/physical/`.** Reusado por todo ticket que introduza
  parâmetro: exige `not_canon: true`, versão, proveniência, `evidence_sufficiency` próprio e marcação
  `[INT]`.
- **`population-prior.yaml`.** `status: PROVISIONAL`, hipótese nula de seletividade de coorte
  declarada (coorte nacional **sem** deslocamento, enquanto `oq.capability.cohort-selectivity` estiver
  aberta), cargas fatoriais marcadas como suposição (S2), médias e DP provisórios (S1).

## Arquivos e módulos

```
pyproject.toml                                   novo
src/embodiment/{__init__,rng,types,prior,seeding,snapshot,eventlog,cli}.py
data/models/physical/population-prior.yaml       novo
data/models/physical/schema/model-file.schema.json  novo (cabeçalho comum)
tests/embodiment/{test_rng,test_prior,test_seeding,test_snapshot,test_architecture}.py
tests/embodiment/invariants/test_invariant_{01,03,08,09,11,13}_*.py
```

## Decisões que consome

13.4 — Python vale para este subdomínio (ADR 0007).

## Testes determinísticos

- **Cópula preserva marginais.** Amostrar N grande e verificar que a marginal por dimensão bate com a
  do arquivo dentro de tolerância declarada.
- **Estrutura de correlação existe (F7).** A correlação empírica entre `max_strength` e `body_mass` é
  positiva e a entre `body_mass` e `aerobic_capacity` relativo é negativa, com o sinal das cargas
  declaradas. Testa-se **que a estrutura existe**, não que ela está certa — enquanto S2 estiver
  aberta, as cargas são suposição declarada e o teste de calibração fica `xfail` com motivo (spec
  §12).
- **Sem aluno de cauda conjunta absurda (F7).** Nenhum aluno da coorte fica simultaneamente acima do
  percentil 95 em `body_mass`, `sprint_speed` e `aerobic_capacity`.
- **Imutabilidade da amostra (F5).** Duas leituras do mesmo `character_id` no mesmo run devolvem o
  mesmo `capacity_baseline`; qualquer tentativa de reescrita levanta erro.
- **Escrita única (F3).** Um teste de arquitetura falha se `capacity_baseline` for atribuído fora de
  `seeding.py`.
- **Metadados obrigatórios.** Run sem `world_seed`, sem `prior_version` ou sem
  `posterior_hash_by_character` falha **ao iniciar**, com mensagem que nomeia o campo faltante.
- **Validador de comparação como entrada (F4).** Um arquivo de parâmetro ou fixture que ordene dois
  personagens é recusado; a única ordenação admissível é `COMPARATIVE` ancorada a um evento observado
  (invariante 8).
- **Unidade física obrigatória (invariante 9).** Toda dimensão declara unidade; `pain_tolerance`, a
  única sem unidade, declara a coorte de referência.
- **Testes de arquitetura (AC 12).** `src/embodiment/` não importa cliente de LLM; nenhuma chamada a
  `numpy.random.<fn>` ou `random.<fn>` no pacote.
- **Lint de repositório (AC 11).** Nenhum `capacity_posterior` de personagem canônico está commitado
  e nenhum perfil de capacidade nominal compila.

## Property tests

- **P5 (parcial) — isolamento de substream.** Para qualquer conjunto de personagens, acrescentar um
  que não participa de nenhum evento não altera nenhum sorteio dos demais. É este teste que justifica
  o esquema de substreams existir; sem ele o esquema é cerimônia.
- **P6 (parcial) — determinismo.** Mesmo seed, mesmo log de seeding, após normalizar timestamps de
  parede.

## Cenários

**7 — NPC sem nenhuma evidência.** Um NPC criado depois que a coorte já foi semeada sorteia de um
substream derivado de `hash(character_id, world_seed)`, obtém o mesmo corpo que obteria se tivesse
sido criado primeiro, e não desloca ninguém (F6).

## Checagens de fronteira de conhecimento

Nenhuma ainda — não há agente nem contexto neste ticket. A regra que este ticket **estabelece** é
estrutural: `capacity_baseline` e `posterior_hash` vivem no snapshot e no event log, que são
artefatos de engine. Nenhum deles tem caminho para prompt, e nenhum é documento recuperável por RAG.

## Fora do escopo

`BodyState` e qualquer dinâmica (PSV1-2). Estimador e restrições (PSV1-3). Deriva de
`capacity_baseline` (fora da V1 inteira). Calibração numérica do prior — S1 e S2 continuam abertas, e
o arquivo diz isso em vez de fingir.

## Evidência de conclusão

1. O comando da seção de resultado roda e escreve `runs/demo/{events.jsonl,snapshot.yaml}`.
2. Executar duas vezes com o mesmo seed produz `events.jsonl` idêntico após normalização.
3. `events.jsonl` contém 40 registros `capacity.sampled`, cada um com `posterior_hash`,
   `prior_version` e `evidence_sufficiency` por dimensão — todos `0.0`, porque não há evidência.
4. Um relatório de correlação da coorte amostrada, no log ou em artefato do run, exibindo os sinais
   esperados das cargas.
5. Suíte verde, incluindo os testes de arquitetura e os `xfail` de calibração **com motivo escrito**.
