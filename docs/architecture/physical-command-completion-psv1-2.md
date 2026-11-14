# PSV1-2 — prova versionada de conclusão física, não WorldRevision

**Issue #107, PR #109.** Consumidor do contrato A2: issue #99, PR #104 já integrada. **Não implementa PSV1-8 integral, nem Go CommitCoordinator, nem fonte autorizada A1.**

## 1. Problema corrigido

Antes desta fatia, um log JSONL que terminasse em um `body.advanced` fisicamente consistente poderia ser um **prefixo de um comando interrompido** e seria impossível distingui-lo de uma operação que terminou normalmente. Os hashes de snapshot e log não diziam quais intervalos o produtor **pretendia** executar. A2 marcava todo resultado como `UNATTESTED_PREFIX`.

A CLI PSV1-2 agora escreve, dentro da mesma fase `run.extended`:

1. `body.advance.intent.v1` (antes do primeiro intervalo): protocolo fixo `psv1-2.advance.v1`, ator, dias pretendidos, contagem esperada de registros, `sleep_hours`, `quality`, `wbgt_c`, `t_hours_before` e `dynamics_version`.
2. Exatamente `expected_event_count` registros `body.advanced`, com clock, delta, ambiente, carga e `channel_diff` verificados pela PSV1-2.
3. `body.advance.complete.v1` **somente após todos os intervalos**: protocolo, ator, `intent_seq`, `first_advance_seq`, `last_advance_seq`, `planned_days`, `emitted_event_count`, `t_hours_after`, `dynamics_version` e `final_body_sha256` (SHA-256 da serialização JSON de referência de `as_document(BodyState)`).
4. `os.fsync` do descritor do log após a escrita do recibo terminal e antes do retorno de sucesso.

**Um intervalo não recupera o comando perdido**: um prefixo com intenção e parte dos avanços permanece fisicamente analisável, mas `all_phases_complete=False`. Um comando interrompido não pode ser “completado” por outra operação e um log legado sem intenção/recibo não é promovido a `ATTESTED_COMPLETE` por retroatividade.

## 2. Algoritmo de verificação

`replay_body_history` valida a origem/snapshot/seed, todos os registros, modelos pinados, autores, sequência e diffs corporais. A seguir, para cada fase versionada, reconstitui a **rotina diária inteira** a partir do plano declarado, conferindo exaustivamente cada `awake` / `asleep`, sua duração e seu ambiente, os intervalos reais e a posição do marcador final. Somente o digest do **BodyState recomposto** pode satisfazer o terminal.

Campos/contagens extras, dados fora do tipo, versões desconhecidas, troca de ator, reorder, marker duplicado, diff divergente, fragmentos inválidos ou início de outra fase sobre intenção incompleta falham fechado. Fases legadas sem intenção podem ser analisadas como prefixos mas **nunca satisfazem o gate de conclusão**. O escritor recusa adicionar uma nova fase quando encontra um prefixo anterior sem conclusão verificável.

Para cada run, a continuidade retorna:

- `all_phases_complete`: true somente para pelo menos uma fase concluída, sem fase legado/incompleta;
- `completed_phase_count` e `completed_transition_count`: contagens verificadas das fases fechadas;
- `bodies`: estados reproduzidos do prefixo, **ENGINE_ONLY** mesmo em caso de falha de completion.

A2 projeta a conclusão com `completion=ATTESTED_COMPLETE` ou `UNATTESTED_PREFIX`; seu schema fechado inclui `command_receipts[]` com ids/seq locais do intervalo, contagem de dias, hash do corpo terminal e digest do registro terminal normalizado. **Esses campos não são uma proposta de EventOrderKey, LogicalSequence ou SimulationInstant**.

## 3. O que o recibo NÃO prova

- **Autenticação do produtor:** um adversário que pode reescrever ambos os arquivos pode recalcular os hashes e forjar uma história autoconsistente. Só um corte de origem autorizado, coerente e imutável executado/supervisionado pelo host A1 impede aceitar bytes não autorizados.
- **Atomicidade do append como commit global:** `fsync` torna o marcador escrito mais durável no arquivo; não substitui transação SQLite, não faz `CycleCommit` e não move `WorldRevision`.
- **PSV1-8 completa:** não gera checkpoint retomável de corpo/lesões/observações nem prova modelos físicos ainda não implementados. A primeira transição integrada exigirá o registro físico no bundle Go, seu reducer, a matriz A3 e prova A4, todos no caminho #108/CSFRV1-10.
- **Segredo:** os campos numéricos de BodyState ficam exclusivos do engine, proibidos em agent context; A3 deve verificar cada campo/recipient antes de qualquer projeção qualitativa.

## 4. Testes e gates

Fazer tests reais de `seed_cohort_command` seguido de `advance_clock_command`, alternando 0/24/4/8/0.1/7.3 h de sono, dias e atores; testar crash logo após intent, após uma transição, e antes de footer; manipular planos e recibos sem confiar em seus hashes. Um terminal falseado nunca torna prefixo concluído.

- [x] CI focal #107 com 250 testes, continuidade PSV1-2 150, A2 182, A1 67 no SHA `65f8a4c5ad9a1714ad4c06b7862e921b5f88943c` (GitHub Actions #43542580771, #43542580847, #43542580808, #43542580862); warnings 0. Após edição documental, recheck no HEAD final.
- [x] Revisão adversarial pelo executor: PHY-C01–C03 corrigidos/regressões verdes; PHY-C04 é dependência externa A1/Go, não licença para marcar world truth. Re-review sem outros achados acionáveis no escopo da conclusão PSV1-2.
- [ ] Revisão formal independente por terceiro e merge humano, separados.
- [ ] Admissão real e CommitCoordinator físico em Go #108 ficam pendentes; nenhum write SQLite alternativo.
