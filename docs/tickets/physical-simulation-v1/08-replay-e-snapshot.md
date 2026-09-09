# PSV1-8 — Replay byte-a-byte e snapshot completo

**Spec:** §3.1 (`SNAPSHOT`, `RUN_METADATA`) · §5.3 · §9 · §10 · §14.8 · §14.9
**Modelo:** §13 (determinismo e reprodutibilidade)
**Bloqueado por:** PSV1-6, PSV1-7
**Bloqueia:** —

## Resultado observável

```
python -m embodiment replay --run runs/demo/ --out runs/demo-replay/
diff runs/demo/events.jsonl runs/demo-replay/events.jsonl   # vazio
```

Um run completo — semear coorte, avançar dias, resolver corrida com observadores, resolver disputa,
salvar — é reexecutado a partir do seed e da configuração, e produz o **mesmo event log**, após
normalizar timestamps de parede. E:

```
python -m embodiment snapshot --run runs/demo/ --save s1.yaml
python -m embodiment resume --snapshot s1.yaml --continue-days 2
```

carrega o corpo, as lesões e as crenças físicas de volta e continua de onde parou.

Este é o critério que a spec chama de objetivo real; os outros cinco comportamentos da V1 são a
superfície pela qual ele é observável.

## Escopo

- **Replay completo.** Reexecução a partir de `world_seed` + configuração + versões de parâmetro,
  cobrindo todos os módulos entregues: seeding, dinâmica, estimador, esforço, observação, detecção,
  interocepção, disputa.
- **Normalização de log** para comparação: timestamps de parede fora, tempo de simulação dentro. O
  critério é **byte-a-byte após normalização**, e a normalização é parte do artefato, não uma
  concessão feita na hora de comparar.
- **Snapshot `snapshot_version: 1` íntegro** (spec §5.3): `characters.<id>.capacity_baseline`,
  `characters.<id>.body_state` completo — incluindo `illnesses: []` sem dinâmica — e a árvore separada
  `physical_beliefs` com `self_physical_model` e `capacity_beliefs` por holder. Round-trip com hash.
- **Compatibilidade para a frente** (spec §10):
  - carregar um snapshot cuja `prior_version` seja desconhecida é **erro**, não aviso — os números do
    corpo foram amostrados sob aquele prior e não significam a mesma coisa sob outro;
  - o mesmo vale para as demais versões de parâmetro registradas em `run_metadata`;
  - quando o prior for recalibrado, snapshots antigos **não são migrados**: são reexecutados a partir
    do seed. A amostra não é convertível, e o carregador diz isso na mensagem de erro.
- **Suíte de cenários como um todo.** Os oito cenários do modelo §14, implementados ao longo dos
  tickets anteriores, rodam sob replay e produzem logs idênticos entre execuções.
- **Endurecimento dos metadados obrigatórios.** O run falha ao iniciar sem `world_seed`,
  `prior_version`, `estimator_version`, `dynamics_version`, `contest_resolver_version`,
  `observation_params_version` ou `posterior_hash_by_character`. Junto vão, por personagem,
  `evidence_sufficiency` por dimensão e o ESS do estimador — porque um posterior com sufficiency ≈ 0
  é o prior com outro nome, e quem lê o run precisa saber disso sem reconstruir a inferência.

## Arquivos e módulos

```
src/embodiment/{snapshot,eventlog,cli}.py
src/embodiment/replay.py                         novo
tests/embodiment/{test_replay,test_snapshot_roundtrip,test_version_gating}.py
tests/embodiment/properties/{test_p5_substream_isolation,test_p6_determinism}.py  (ampliados)
tests/embodiment/scenarios/test_all_scenarios_replay.py
```

## Decisões que consome

13.5 — `illnesses` serializado sem dinâmica, confirmado no PSV1-0 e materializado aqui no
`snapshot_version: 1`.

## Testes determinísticos

- **Replay byte-a-byte** de um run que exercita todos os módulos, não só o seeding.
- **Round-trip de snapshot.** Salvar, carregar, salvar de novo produz o mesmo hash. Continuar dois
  dias a partir do snapshot produz o mesmo estado que ter rodado direto sem salvar.
- **Separação de árvore preservada na serialização.** Crença nunca aterrissa em `characters.<id>`,
  nem na ida nem na volta (invariante 1).
- **Versão desconhecida é erro.** Carregar com `prior_version` desconhecida levanta erro que nomeia a
  versão e a razão; um teste falha se virar aviso.
- **Metadados obrigatórios.** Um teste por campo: remover cada um faz o run falhar **ao iniciar**, com
  mensagem que nomeia o campo.
- **`evidence_sufficiency` e ESS presentes por personagem** no `run_metadata` de todo run.
- **Os oito cenários sob replay.** Cada um roda duas vezes e produz o mesmo log.

## Property tests

- **P5 — isolamento de substream, agora sobre o run completo.** Para qualquer conjunto de personagens,
  acrescentar um que não participa de nenhum evento não altera nenhum sorteio de nenhum outro — não só
  no seeding, mas através de dinâmica, esforço, observação, detecção e disputa. É este teste que
  justifica o esquema de substreams nomeados existir.
- **P6 — determinismo, agora sobre o run completo.** Mesmo seed, mesma configuração, mesmas versões de
  parâmetro → mesmo log.

## Checagens de fronteira de conhecimento

- **O event log contém números que nunca entram em prompt.** É o artefato de auditoria da spec §9, e
  este ticket o consolida: um teste verifica que nenhum caminho de contexto lê `events.jsonl`.
- **O snapshot contém world truth e crença lado a lado**, em árvores separadas e com donos
  explícitos. O carregador não tem função que devolva as duas fundidas.

## Fora do escopo

Migração de snapshots — não existe nenhum snapshot persistido, o repositório não tinha código antes
desta spec, e a regra estabelecida é que snapshots antigos são **reexecutados**, nunca convertidos.
Integração com `ExamSpec`, que é o consumidor seguinte e não faz parte da V1.

## Evidência de conclusão

1. `diff` entre run e replay vazio, para um run que exercita seeding, dinâmica, esforço, observação,
   detecção, interocepção e disputa.
2. `resume` a partir de snapshot continua com corpo, lesões e crenças intactos.
3. Carregar snapshot com versão de parâmetro desconhecida falha com erro nomeado.
4. P5 e P6 verdes sobre o run completo.
5. Os oito cenários do modelo §14 verdes e reprodutíveis sob replay — fecha o AC 9.
6. Um relatório de run mostrando, por personagem, `evidence_sufficiency` por dimensão e ESS.
7. **Revisão final contra os doze critérios da spec §14**, item por item, anexada ao ticket. É aqui
   que a V1 é declarada pronta, ou não é.
