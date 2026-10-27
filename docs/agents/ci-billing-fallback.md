# Política global — falha de CI/runners e possível billing

Este documento aplica-se a **todas** as skills canônicas, wrappers Claude, subagentes, coordenadores, revisões e handoffs de `cote-ai-simulation`. A skill da fase não pode contradizer esta política.

## Evidência de indisponibilidade

- Consulte o run do GitHub Actions e os jobs. `runner_id=0` e `steps=[]` com falha imediata significam **CI_NOT_RUN**, não regressão nem comprovação isolada de problema financeiro. Só classifique `BILLING_CONFIRMED` com evidência de quota/billing fornecida pelo GitHub ou pelo proprietário.
- Caso o job execute passos e falhe, registre a falha como `TEST_FAILED`/erro real com logs. Um scanner não disponível por licenciamento é `ENTITLEMENT_UNAVAILABLE`, não sucesso.
- Não desabilite checks, retire proteção, ajuste billing, contrate runner ou produza status sintético para obter PR verde.

## Provas alternativas na branch isolada

- Rode validações **locais proporcionais e efetivas**: testes determinísticos, replay/seed, integridade de world-state e ledgers, migração/fork, barreiras de conhecimento, mutation checks e avaliações versionadas pertinentes. Capturar comando, commit/HEAD, exit code, resumo e lacunas; não replicar suites verdes sem mudança relevante.
- LLM-evals não substituem testes determinísticos; avaliações estocásticas precisam de versão de modelo/config, seed/substream, dataset/slice, repetição e limitações. Sem quota, classifique `NOT_RUN`; nunca atribua scores inventados.
- Use infraestrutura descartável de propriedade do teste. Preserve Docker, bancos, volumes e ambientes pessoais; marque `VALIDATION_BLOCKED` se isolamento/recursos indisponíveis.
- Proíba colar corpus protegido de novels, PDF não autorizado, personagens privados, segredos, prompts com dados sensíveis ou material de licença não esclarecida em Issues, logs, PRs ou provedores de IA.

## Relatório e autoridade

Reportar por verificação `LOCAL_PASS`, `LOCAL_FAIL`, `CI_NOT_RUN`, `EVAL_NOT_RUN` ou `VALIDATION_BLOCKED`; mencionar evidência concreta e impacto do que ficou sem comprovação. A exceção de CI não autoriza merge, publicação de packs como cânone verificado, ativação da simulação ou release. O usuário autoriza transições de fase conforme `delivery-pipeline`, e a aprovação de merge continua humana. Se quota acabar, registre checkpoint Git-first e pare, sem downgrade silencioso de modelo.

Checklist: Issue/fase/PR, base+HEAD SHA, link do run/runner/steps, comandos/exit codes, negativos/mutantes/replay/ledger, avaliações faltantes, findings, privacidade, cleanup e autorização ainda necessária.
