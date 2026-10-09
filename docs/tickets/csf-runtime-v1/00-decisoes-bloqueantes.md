# CSFRV1-0 — Incorporar decisões aceitas e liberar o runtime

**Spec:** §14  
**Bloqueado por:** —  
**Bloqueia:** CSFRV1-1 e CSFRV1-9A
**Natureza:** pesquisa/arquitetura/documentação; nenhuma implementação do runtime.

## Resultado observável

A spec passa de `Proposed` para `Accepted`, com os incrementos 1–9 liberados pelos ADRs 0010/0011 já
aceitos e o incremento 10 explicitamente bloqueado apenas pelo futuro contrato do adapter físico.
O plano referencia Go 1.27.2, SQLite/modernc e seus caminhos concretos; nenhum implementador repete
pesquisa encerrada ou cria ADR concorrente.

## Escopo

1. **Stack (§14.1).** Referenciar o ADR 0010 aceito: Go 1.27.2, core atrás de ports, segundo runner
   independente e fronteira de processo/dados com Python `src/embodiment/`.
2. **Persistência (§14.2).** Referenciar o ADR 0011 aceito: um SQLite/modernc por run/workspace,
   WAL + `synchronous=FULL`, BLOBs canônicos e transação local única.
3. **Adapter físico (§14.3).** Registrar sem ambiguidade que continua aberto e bloqueia somente o
   incremento 10. O contrato, schemas e fixtures pertencem ao CSFRV1-9A, que pode ocorrer em paralelo.
4. Atualizar status e §14 da spec, índice e tickets com links, toolchain, dependências e paths reais.

## Arquivos e módulos

```text
docs/research/csf-runtime-and-persistence-v1.md                referência existente
docs/adr/0010-csf-causal-engine-stack.md                       referência existente
docs/adr/0011-csf-causal-persistence-atomicity.md              referência existente
docs/spec/causal-simulation-foundation-v1.md
docs/tickets/csf-runtime-v1/*.md
```

## Testes/validação determinística

- Links da spec resolvem para os paths reais dos ADRs 0010/0011 e da pesquisa.
- A spec aceita nomeia Go 1.27.2 e SQLite/modernc por referência, sem copiar política divergente.
- Busca documental falha se §14.3 aparecer como resolvida antes do CSFRV1-9A ou bloquear 1–9.
- Todos os módulos previstos usam `internal/csf/`, `cmd/` e testes Go; o segundo runner tem path
  explícito e independente.

## Evals e fronteira de conhecimento

Não há eval comportamental. Este ticket apenas preserva no CSFRV1-9A a obrigação futura de definir a
allow-list física e executá-la no CSFRV1-10.

## Fora do escopo

Repetir a pesquisa de stack/persistência; criar ADRs substitutos; desenhar/implementar o adapter;
escolher framework de agentes/provedor de LLM; alterar semântica do ADR 0008.

## Evidência de conclusão

1. ADRs 0010/0011 e pesquisa estão presentes na branch e ligados pela spec.
2. Spec está `Accepted` para 1–9 e declara §14.3 como gate exclusivo do incremento 10.
3. Grafo mostra CSFRV1-9A paralelo e dependência exclusiva de CSFRV1-10.
4. Plano usa Go 1.27.2, SQLite/modernc e paths concretos, sem placeholders.
5. Diff exclusivamente documental, sem runtime de produção ou spike físico prematuro.
