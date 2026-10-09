# CSFRV1-8 — Replay, snapshot/resume e fork verificáveis

**Spec:** §6.6 · §7.8 · §9 (resume/versionamento) · §10.1 · §11.2 · §12  
**ADR 0008:** invariantes 15, 27 e revalidação de 24/29/32/35–39  
**Bloqueado por:** CSFRV1-7  
**Bloqueia:** CSFRV1-9

## Resultado observável

Um run que inclui conflito de recurso, trigger, evento perceptível e comunicação em trânsito é
executado continuamente, por causal replay e por snapshot+resume em diferentes crash points. As
três rotas produzem os mesmos bytes autoritativos, revisões, cursores e state hash. Um fork abre novo
`run_id` a partir do checkpoint verificado e tails diferentes nunca compartilham evento.

## Escopo

- Causal replay de genesis + journal + eventos referenciados, incluindo commits vazios.
- Snapshot V1 com manifesto/seed/hash, world/clock, cinco prefixos/cursors, pendências derivadas,
  trigger runtime e refs de checkpoints epistemológicos por ator elegível.
- Checkpoint epistemológico opaco: hash dos bytes exatos/locator; sem regenerar via modelo.
- Resume reconcilia control state, outbox/evidence/barrier e work in-flight antes de novo ciclo.
- Snapshot+tails, policy/version gating e falha fechada em corrupção/artefato ausente.
- Fork same-policy e cross-policy via `parent_checkpoint_history_ref`; nenhum merge.
- Failure-injection harness ampliado para fence, retry, commit, percepção, transmissão e snapshot.

## Arquivos e módulos prováveis

```text
internal/csf/{replay,snapshot,resume,fork}/
internal/csf/{replay,snapshot,resume,fork}/**/*_test.go
tests/failureinjection/end_to_end_test.go
tests/properties/{determinism,parallelism,substream_isolation}_test.go
evals/simulation/csf-runtime-v1/
```

## Testes determinísticos

- Replay recalcula event id/order/sequence/digests/revision/state hash antes de reducer.
- Commit vazio avança revision/ordinal/sequence somente pelo journal; event store segue vazio.
- Snapshot round-trip mantém bytes/hash; tails após resume igualam execução contínua.
- Snapshot em `HALTED_ON_ABORT` continua parado; entre retry/fence abre a tentativa autorizada;
  fence in-flight reutiliza plano/corte sem chamar seleção de coordenada.
- Snapshot no meio de transmission entrega exatamente uma vez no mesmo instante causal.
- Task parcialmente processada é reconciliada antes de novo dispatch.
- Policy/schema/reducer/checkpoint ausente ou hash divergente impede replay/resume.
- Replay/resume não importam cliente de LLM nem consultam wall clock.
- Fork preserva prefix/hash parental e isola ids/events pós-fork.

## Property tests e failure injection

Para os mesmos fatos persistidos, variar paralelismo, worker order e crash point mantém ledgers e
snapshot byte a byte. Adicionar ator/avaliação irrelevante não desloca substreams. Cada fronteira
recupera prefixo anterior ou unidade completa e idempotente.

## Evals e checagens de conhecimento

- Determinism eval compara todos os ledgers/snapshot, não apenas estado final.
- Simulation eval executa seeds, paralelismos e crash points variados.
- `knowledge-boundary-audit` verifica que checkpoint opaco não é aberto pelo core, contexto não lê
  snapshot global e rumor restaurado conserva provenance de claim sem criar world truth.

## Fora do escopo

Migrar automaticamente snapshot físico antigo ou schema incompatível; merge de forks; regenerar
belief/memória por LLM.

## Evidência de conclusão

1. Relatório compara execução contínua, replay e múltiplos resumes: zero diferença autoritativa.
2. Matriz de crash points fica verde e indica estado recuperado em cada fronteira.
3. Forks têm mesmo parent ref, tails isolados e nenhum id/event cruzado.
4. Testes falham fechado para cada versão/hash/checkpoint removido ou corrompido.
