# CSFRV1-0 — Fechar decisões bloqueantes e aceitar a spec

**Spec:** §14  
**Bloqueado por:** —  
**Bloqueia:** CSFRV1-1 e, transitivamente, todo código de produção  
**Natureza:** pesquisa/arquitetura/documentação; nenhuma implementação do runtime.

## Resultado observável

A spec passa de `Proposed` para `Accepted` depois que três decisões rastreáveis definem: stack da
fundação, topologia de persistência/atomicidade e interface versionada do adapter físico. Cada
ticket seguinte encontra linguagem, toolchain, fronteira transacional e contrato físico explícitos;
nenhuma dessas escolhas nasce por acidente no primeiro commit de código.

## Escopo

1. **Stack da fundação (§14.1).** Pesquisa curta compara as opções apropriadas ao Simulation Engine,
   escolhe linguagem/runtime/toolchain e fixa a fronteira de processo/dados com Python
   `src/embodiment/`. A decisão não escolhe framework de agentes, LLM, RAG ou UI.
2. **Persistência e atomicidade (§14.2).** Pesquisa e ADR comparam ao menos transação única no mesmo
   banco e protocolo reconciliável por `fence_digest` entre stores. A escolha cobre isolamento,
   CAS, snapshot, retenção, backup, recovery e failure injection e demonstra que nenhum consumidor
   atravessa commit incompleto.
3. **Adapter físico (§14.3).** Contrato versionado mapeia JSONL/snapshot do `Embodiment` para inputs,
   typed values e eventos globais. `seq` e `sim_time` locais nunca viram sequência/instante globais;
   o pacote físico não recebe port de commit nem publica revisão.
4. Atualizar a §14 com links/linhas `**Decidido:**`, mudar o status da spec para `Accepted` e
   substituir placeholders de paths/tooling deste conjunto pela stack decidida.

## Arquivos e módulos

```text
docs/research/csf-runtime-stack-and-persistence.md             novo
docs/adr/0010-csf-runtime-stack.md                             novo
docs/adr/0011-csf-persistence-and-atomicity.md                 novo
docs/architecture/embodiment-causal-adapter-v1.md              novo
docs/spec/causal-simulation-foundation-v1.md
docs/tickets/csf-runtime-v1/*.md
```

Os números de ADR são indicativos: usar os próximos ids livres no momento da execução.

## Testes/validação determinística

- Protótipo descartável de atomicidade injeta falha em cada fronteira candidata e prova estado
  anterior completo ou commit completo, nunca mistura.
- Spike de fronteira física converte fixtures existentes duas vezes e produz bytes idênticos, sem
  importar `src/embodiment` quando a stack escolhida exigir processo separado.
- Checagem documental falha se a spec estiver `Accepted` sem links para as três decisões.
- Checagem de arquitetura proposta no ADR mostra como impedir write ports fora do
  `CommitCoordinator` e no `Observatory`.

## Evals e fronteira de conhecimento

Não há eval comportamental. O contrato do adapter enumera explicitamente os campos que jamais podem
virar `Observation`/`KnowledgeInput`: `BodyState`, capacidade/posterior, RNG, telemetria e estado de
terceiro. Essa allow-list será executada no CSFRV1-10.

## Fora do escopo

Implementar a stack escolhida; migrar JSONL físico; escolher framework de agentes/provedor de LLM;
alterar semântica, codec, invariantes ou atomicidade do ADR 0008. Alternativa que exija enfraquecer
esses contratos reabre o ADR, não fecha este ticket.

## Evidência de conclusão

1. Três decisões aceitas e ligadas pela spec.
2. Matriz de alternativas e protótipos reproduzíveis anexos à pesquisa.
3. Spec com status `Accepted` e zero decisão aberta na §14.
4. Este conjunto atualizado com comandos e paths concretos da stack escolhida.
5. Diff exclusivamente documental/protótipos descartáveis, sem runtime de produção.
