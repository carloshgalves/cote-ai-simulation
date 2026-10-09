# CSFRV1-10 — Adapter físico e cenário causal de referência

**Spec:** §1 · §3.1 (integração física) · §11.3–11.5 · §12.2  
**ADRs:** 0006, 0007, 0008, 0010 e contrato do CSFRV1-9A
**Bloqueado por:** CSFRV1-9, CSFRV1-9A e entregas físicas consumidas pelo contrato
**Bloqueia:** primeira integração de `ExamSpec`/Agent Cognition; não autoriza merge por si só

## Resultado observável

Um único comando do harness executa o cenário da spec §1: duas fontes fechadas, dois rounds com
respostas invertidas, disputa da última unidade, commit atômico, avanço/trigger, claim verdadeiro ou
falso, segredo isolado, abort+retry autorizado, crashes/resume, replay e Observatory. Uma resolução
física real atravessa o adapter versionado e vira evento/revisão global sem copiar telemetria para o
agente. O relatório final prova bytes/digests/state hash idênticos entre execução contínua,
paralelismos e resumes.

## Escopo

- Implementar o contrato `Embodiment` → CSF fechado no CSFRV1-9A como fronteira de dados/processo.
- Verificar hash/version do snapshot/log físico de origem e mapear apenas records suportados.
- Derivar typed payload, candidate, event provenance/order e reducer global; somente o
  `CommitCoordinator` publica revisão.
- Preservar `seq`/`sim_time` físicos como provenance local quando útil, nunca como sequence/instant
  globais; não promover runs antigos a histórico causal.
- Adapter de percepção física produz somente output qualitativo/endereçado permitido.
- Harness integrado, matriz rastreável das 41 invariantes e todos os cenários de stress.
- Runbook de operação, policy/schema docs, backup/restore e diagnóstico de parada.

## Arquivos e módulos prováveis

```text
internal/csf/adapter/embodimentv1/
internal/csf/extensions/embodimentv1/
internal/csf/adapter/embodimentv1/**/*_test.go
tests/scenarios/reference_run_test.go
tests/invariants/invariant_01_*.go ... tests/invariants/invariant_41_*.go
evals/{simulation,determinism,knowledge-boundary}/csf-runtime-v1/
docs/operations/csf-runtime-v1.md
```

## Testes determinísticos

- Mesmo input físico/hash produz mesmo candidate/event/revision; byte alterado falha fechado.
- `BodyState`, capacidade/posterior, RNG e telemetria nunca aparecem em evidence/context.
- Adapter não importa/recebe committer, mutable clock ou world store; não publica revisão.
- Sequence/time local não decide order global; permutar records locais suportados mantém projeção
  quando a semântica set-like permitir.
- Snapshot físico incompatível/ausente permanece legível pela ferramenta original, mas é recusado
  como snapshot causal.
- Cada uma das 41 invariantes possui teste nomeado e link no relatório.
- Cada cenário de stress do ADR possui teste de integração/propriedade nomeado e owner.

## Evals obrigatórias

- **Simulation:** cenário completo sob seeds, paralelismos e crash points variados; causalidade,
  lifecycle e resultado verificados.
- **Determinism:** comparação byte a byte de genesis, cinco ledgers, agenda/runtime, snapshot e
  state hash entre execução contínua/replay/resume.
- **Embodiment integration:** mesma resolução gera mesmo envelope/revisão e consequência física
  persiste entre ciclos.
- **Knowledge boundary:** inspeção por destinatário tenta segredo, POV alheio, canon futuro,
  telemetria física e Observatory privilegiado.
- LLM/RAG evals continuam não aplicáveis; um contract eval de responder futuro pertence a outra spec.

## Fora do escopo

Escolha por LLM, crença/reflexão, regras de exame, ingestão de feats, refazer o modelo físico,
converter automaticamente histórico JSONL anterior ou declarar produção distribuída além da
topologia decidida.

## Evidência de conclusão

1. Comando documentado gera run, snapshot e relatório causal verificável sem LLM.
2. Execução contínua, ordens de worker opostas, replay e resumes têm zero diff autoritativo.
3. Failure injection não encontra estado parcialmente publicável.
4. Matriz 41/41 e todos os cenários do ADR apontam para testes verdes.
5. Relatórios de simulation/determinism/embodiment/knowledge-boundary eval ficam versionados.
6. Observatory consulta o resultado sem mudança de hash.
7. Runbook diagnostica closure wait, rejection, abort, crash in-flight, barrier e version failure sem
   consultar memória do processo.
8. Revisão final contra os dez itens da Definition of Done da spec §11.5 fica anexada e sem lacuna.
