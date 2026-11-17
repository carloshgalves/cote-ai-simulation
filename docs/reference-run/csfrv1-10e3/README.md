# CSFRV1-10/E3 — autorização cognitiva após snapshot/resume

Issue #142, pai #119. E2 #141 concluída pelas PRs #146/#147.

## Duas autoridades distintas

`VerifySnapshotV1` continua verificando raízes históricas válidas, inclusive um fence congelado, HALTED, RETRY e transmissão em trânsito. Esse resultado **não** constitui autorização cognitiva. `OwnerWorkReady` é uma projeção verificada no mesmo read transaction: snapshot e tip IDLE, mesma revisão, sem tarefas pendentes no corte selado ou no tip, prefixo de evidence byte-exato e roster dos holders/actors/receipts ainda válido na autoridade corrente. Alterações de admission ou percepção não precisam incrementar a revisão para revogar autorização. `Gate.RestoreAndAcknowledge` exige a projeção antes de solicitar restore e novamente após o ACK; `RunWork` revalida antes de chamar o callback. Uma sessão nova não herda permissões de um ACK persistido.

`CreateSnapshotV1AtRevision` conserva o contrato de revision pin e IDLE sob writer lock, e agora exige ausência de percepção pendente. `localdemo.Host.StageSnapshot` usa essa entrada. Um snapshot genérico para recuperação causal pode ser histórico/frozen e não pode ser promovido a um corte cognitivo nem reescrito após mudança no mesmo revision. Uma tentativa de stage pode deixar receipt órfão (ordem durável do B3), mas não cria snapshot ou permit.

## Matriz e critérios

- RED C4: ambos os proprietários restaurados e ACKed; um fence nativo é publicado, falha transacional conserva revisão 6. O snapshot continua verificável; callback e novo restore devem ser recusados.
- C4 joint A/B: duas reaberturas, `ErrNotAcknowledged` antes de cada restore, troca de receipt e snapshot inválido revogam permit; comparação integral das três bases com oráculo E1. O claim entregue a B não chega a A.
- Transmissão em trânsito: snapshot de owner pinado recusa percepção pendente; snapshot de recovery preserva tarefas. Drains repetidos não duplicam completions ou evidence. Snapshot selado quando pendente jamais adquire autoridade cognitiva pelo drain posterior no mesmo revision.
- D3: duas fontes A1 genuínas no mesmo RunID, bloqueio de restore histórico de C4 após três commits, novo checkpoint conjunto sintético no corte 9; cinco ledgers e cursores de source intactos, segundo cold restore byte-exato.
- Corrupção direcionada de assinatura de escrow, versão de snapshot, versão e conteúdo privado do owner, além de receipt de outro ator.
- Teste de ACK atravessando alteração da autoridade no mesmo revision: ACK poderia ter persistido, mas não concede permit.

## Limites

Trata-se de gate local de host cooperativo, não de isolamento de um LLM arbitrário nem de transação distribuída entre owner e SQLite. A verificação não mantém um writer lock durante o callback. Os testes não autorizam retomada interna de Python A1, fork E4, permutações E5 ou homologação independente #93/#94/#113. Os checkpoints são sintéticos e privados: não são modelos de cognição nem reconstroem BodyState.
