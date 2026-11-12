# ADR 0013 — Owner cognitivo durável: fronteira de confiança e prova local S1

**Status:** ACCEPTED for **local single-operator demonstrator** — aprovação do operador em 2026-11-12 após merge da PR #75. **Não** aprovado para produção, multi-tenant ou armazenamento remoto.
**Data:** 2026-11-12
**Issue:** [#74 (S1)](https://github.com/carloshgalves/cote-ai-simulation/issues/74), sob [#58 (P0-release)](https://github.com/carloshgalves/cote-ai-simulation/issues/58)
**Baseline inspecionada:** `main@5474413e4a2c43bdb3d89f7298b272d58342157e`
**Normas preservadas:** [ADR 0001](0001-world-truth-and-knowledge-boundaries.md), [ADR 0002](0002-character-canon-rag-and-memory.md), [ADR 0008](0008-causal-simulation-foundation-v1.md), [ADR 0009](0009-canonical-causal-codec-and-digests.md), [ADR 0010](0010-csf-causal-engine-stack.md), [ADR 0011](0011-csf-causal-persistence-atomicity.md), [owner S0](../architecture/cognitive-owner-durable-adapter-s0.md), [recibo R22](../architecture/epistemic-owner-receipt-v1.md).
**Apuração e aceite proposto:** [S1a — inventário e plano](../validation/cognitive-owner-s1a-2026-11-12.md).

## Problema e fato já demonstrado

S0 integrou `internal/csf/ownercheckpoint.Gate` e a interface `DurableOwner`; cenários de falha usam owner simulado. O core CSF consegue verificar uma assinatura Ed25519 ligada a run/ator/cobertura/bytes e persistir escrow + snapshot SQLite. **Não consegue comprovar, só pela assinatura, que o owner externo gravou fisicamente o checkpoint, que os dados sobrevivem ao seu processo/host, que restauração foi efetivamente efetuada nem que uma aplicação evita invocar seu modelo por fora do gate.**

O repository possui executores **determinísticos** de WORK e retomada, mas não um processo de cognição/LLM integrado à superfície de `RunWork`. Não confundir um terminal `CommitWork` com geração de pensamento por um agente.

## Decisão aceita para a prova local S1b (restrita e reversível)

1. **Ownership e limites:** CSF continua autoridade exclusiva de mundo, clock, evidência, journal, cinco ledgers, snapshot, publicação e lineage; owner da cognição, **fora** do core, controla o significado e a persistência de crenças/memória, seu estado privado, a chave privada Ed25519, restauração e confirmação de ACK. O host/orquestrador é responsável por autenticação, autorização, sequenciamento e passagem **obrigatória** pelo gate para cada trabalho cognitivo.
2. **Prova local mínima:** sugerir um **segundo arquivo SQLite independente, pertencente ao owner** (não um segundo arquivo de ledger da CSF), com WAL, `synchronous=FULL` verificados na conexão e tabelas do próprio owner para checkpoints imutáveis e ACKs endereçados. Utilizar bytes sintéticos; não integrar provedor de LLM, conteúdo real de personagem, cloud nem armazenamento compartilhado. Um teste de fechamento/reabertura **em processo distinto**, com falhas entre as etapas, deve demonstrar a propriedade **sob as hipóteses do filesystem local**. A escolha local foi aceita pelo operador antes da implementação B1; qualquer mudança de topologia exige nova decisão.
3. **Fronteira entre dois bancos:** o owner persiste e confirma seu commit **antes** de assinar o recibo; CSF valida assinatura e escrows; snapshot CSF publica depois. Não há transação distribuída owner↔CSF. Checkpoint órfão após falha entre owner-commit e CSF-ref pode ser reutilizado por retry idempotente; não deve ganhar autoridade de mundo nem ser coletado sem prova de retenção/linhagem. Se falhar depois de snapshot, nova sessão restaura o checkpoint exato e exige ACK. Falha ou dúvida = **nenhum novo WORK cognitivo**.
4. **Escopo/autorização:** `ownercheckpoint.Scope` atualmente contém `Run`, `Namespace` e `Actor`, **não** `Tenant`. Um tenant autenticado deve ser ligado à run pelo host usando registro/ACL verificáveis e permissão do ator antes de qualquer chamada a `Stage`, `RestoreAndAcknowledge`, `RunWork` ou leitura privilegiada de escrow. O tenant não é criado por simples string fornecida por cliente; nenhum recibo assinado deve ser confundido com autorização de acesso. **Sem host autenticado, S1b é apenas demonstrador single-operator**.
5. **Locator e chaves:** `CognitiveCheckpointSigningBytesV1` exige locator não vazio compatível com `machine-id` ASCII. Locator é identificador **opaco**, nunca path livre, URL assinada ou credencial; resolverá internamente para um item imutável vinculado a `(run, actor namespace, actor, store, schema, producer, coverage, hash)`. Repetição com mesma identidade+bytes retorna mesmo locator/recibo/ACK; colisão de identidade com bytes diferentes é erro. Chave privada permanece sob owner, **fora do repositório, CSF e logs**; teste pode usar chave efêmera sintética, não prova gestão de chaves de produção. Chave pública é fixada pelo contrato existente e não rotaciona silenciosamente.
6. **Restauração e ACK:** `Restore` autentica escopo/locator, reabre a versão persistida exata, **reativa** estado do owner correspondente, retorna os bytes idênticos ao escrow CSF. `Acknowledge` só registra confirmação durável/idempotente depois da restauração bem-sucedida e vincula `run/actor/receipt/snapshot/content hash`. ACK perdidos ou não persistidos impedem concessão do permit; ACK repetido para o mesmo checkpoint é seguro. Um novo processo começa sempre sem permit em memória, ainda que exista ACK durável antigo.
7. **Dispatch e concorrência:** apenas **WORK cognitivo/modelo** é condicionado ao `Gate.RunWork` exato; operações determinísticas de `DispatchFencedWorkV1` e `ResumeRunV1` permanecem sob a autoridade de seu fence/terminal próprio. Não colocar LLM em replay/reducer. Para a prova local, um único host/dispatcher e um único writer autorizado por run; multi-processo/multi-host exigirá fencing interprocesso e novo aceite. Não assumir que um `RWMutex` local resolve corridas entre hosts ou que a checagem de tip antes do callback congela outras transações do banco.
8. **Fork:** filhos V2 têm novo `run_id`; recibo/owner do pai serve apenas como proveniência histórica autenticada; o filho precisa do próprio checkpoint owner assinado e restaurado antes de WORK novo. `buildCheckpoints` do handoff V1→V2 deve ser tratado como **seam de integração do host** (não como licença para regenerar memória nem para pular `Stage`). Preservar cut exato, lineage, recovery e os cinco ledgers.
9. **Privacidade:** sem passagem de conteúdo de memória para `WorldTruth`, RAG global, logs, auditoria pública, observação de outros atores ou eventos do core. Criptografia em repouso, autenticação do serviço, autorização por tenant, rotação/revogação, política de retenção e backup de chaves ainda exigem desenho/validação próprios antes de memória real ou múltiplos usuários.
10. **Sem garantia fictícia de exactly-once:** idempotência de checkpoint/ACK **não** implica exactly-once de chamadas a LLM externo. O demonstrador usa um callback contabilizado sem efeito externo; o caminho futuro de modelo requer identidade/resultado persistidos, protocolo de retry e avaliação específica.

### Contrato ordenado (ponta a ponta da prova local)

```text
Host autenticado (ou demonstrador local sinalizado) verifica run/actor/ACL
  -> owner.PersistAndSign: grava bytes privados e COMMIT durável, só então assina
  -> Gate.Stage: CSF verifica Ed25519 e escrows recibo+bytes em SQLite/WAL/FULL
  -> host publica CreateSnapshotV1 com mesmo checkpoint/coverage e verifica selo
  -> encerra owner e host; reinicia em novos processos
  -> Gate.RestoreAndAcknowledge: verifica snapshot/escrow/roster/tip
        -> owner.Restore exato + bytes idênticos -> ACK durável e idempotente
  -> Gate.RunWork autoriza apenas callback cognitivo endereçado
```

Qualquer falha entre as etapas é recuperável ou **fail-closed**. A operação owner-commit órfã **não** autoriza snapshot nem WORK.

## Alternativas avaliadas (escolha local aceita; produção pendente)

| Opção | Benefício | Limite para a #74 |
| --- | --- | --- |
| **SQLite exclusivo do owner, local — RECOMENDADO PARA DEMONSTRADOR** | Sem serviço adicional, testes reproduzíveis de commit/rollback/reopen, aproveita toolchain Go/SQLite existente | Mesmo host/disco não fornece backup remoto, replicação, disponibilidade do owner, nem demonstra `fsync` de provedor cloud; exige cuidado com WAL e backup consistente |
| Diretório de blobs imutáveis com temp+rename+fsync | Simples inspeção de bytes e artefatos | Semântica de diretório/fsync, symlinks, concorrência, limpeza e ACK exige protocolo próprio mais complexo |
| PostgreSQL separado | Portas e transações robustas, expansão multiusuário possível | Serviço/segredos/manutenção antes de existir host de cognição; ainda sem atomicidade distribuída com CSF |
| Object storage remoto + KMS | Pode atender persistência remota e isolamento na evolução de produção | Custos, autenticação, latência, semântica de durabilidade e restauração variam por provider; depende de contrato, credenciais, decisão operacional e drills reais |

**Escolha aceita para B1:** SQLite local do owner **para prova com dados sintéticos**; adiar provedor remoto de produção. A #58 **não** fecha com essa prova. Qualquer integração real de serviço, compra, deploy ou credencial exige autorização própria.

## Modelo de falhas e limites de garantia

- **Process crash / perda de processo:** S1b deve provar que um checkpoint que retornou sucesso é restaurável do owner local, que um ACK recém-escrito resiste a reopen e que todo processo perde permissões voláteis. Resultado exige validação com exit codes no ambiente efetivo.
- **OS crash / power-loss:** WAL/FULL e commits duráveis são hipótese de design; é necessário ensaio de falha compatível com ambiente/FS antes de alegar sobrevivência física a corte de energia.
- **Disco/host perdido, corrupção, backup defasado ou chave perdida:** **não coberto** pela prova single-host; exige cópias independentes, restore drill, integridade, proteção e rotação de chave e RPO/RTO definidos.
- **Rede, timeout, confirmação ambígua:** owner deve consultar a mesma identidade persistida em retry; resultado desconhecido = não conceder permit; nunca assinar antes do commit nem reconstruir bytes por LLM.
- **Concorrência:** mesmos `run/actor/cut` e bytes iguais são idempotentes; bytes diferentes, locators ambíguos e ACK de outro snapshot são rejeitados. Competição entre processos é gate pendente para multi-host.
- **Autorizações e divulgação:** requisição cross-tenant/ator/run é rejeitada antes de receber checkpoint; assinaturas não substituem ACL. Um `Store.LoadCognitiveCheckpointV1` privilegiado não deve ser exposto diretamente a cliente.
- **Recuperação após fork:** autenticar pai e filha; nenhuma assinatura do pai certifica memória privada atual do filho.

### Metas operacionais

**S1b demonstrador:** documentar comportamento medido em processo/reopen, sem prometer RPO/RTO temporal. Para commits locais confirmados, o teste deve exigir **nenhuma perda lógica de checkpoint sob crash de processo**; não generalizar para perda do host.
**Produção:** RPO, RTO, retenção de backups, periodicidade de restore drill, tenant isolation, distribuição de chaves, modo multi-host, regionalidade e custo **PENDENTES DE DECISÃO**. Não adotar números fictícios.

## Condições de aceite antes de S1b

- [x] Aprovação humana da topologia de **owner SQLite separado, dados sintéticos** (2026-11-12, por instrução do operador; PR #75 mesclada).
- [ ] Delimitação do host/runner a criar (não existe host produtivo), responsabilização por ACL e escopo da prova single-operator.
- [ ] Aprovação dos invariantes de commit, restore/ACK, crash, fork, concorrência e proteção de keys; lista de invariantes negativos na [matriz S1a](../validation/cognitive-owner-s1a-2026-11-12.md).
- [ ] Confirmação de que S1b não promete durabilidade remota de produção e de que a #58 permanece aberta.
- [ ] Spec/ticket rastreáveis, PR Draft, revisão independente posterior, CI/validação no SHA correto, sem merge automático.

## Fora do escopo desta ADR

Alterar schemas/codec V1/V2, introduzir campos no receipt, escolher LLM ou framework cognitivo, criar mídia/manuscrito, adicionar memória canônica como estado vivido, definir o provedor cloud de produção, implementar a S1b nesta PR documental, ou homologar recuperação externa. Este documento é **proposta de decisão**, não implementação nem aceitação de release.
