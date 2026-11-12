# COTE AI Simulation

Simulação multiagente inspirada em *Classroom of the Elite*, com personagens persistentes, conhecimento isolado por agente, exames especiais determinísticos e uma linha do tempo capaz de divergir do cânone.

## Princípios

1. **O motor da simulação é a fonte da verdade.** LLMs propõem ações; não arbitram pontuação, regras, posse de itens, presença física ou fatos secretos.
2. **Personagem não é prompt.** Cada personagem combina perfil canônico, evidências comportamentais recuperáveis, crenças, memória episódica, relações e objetivos dinâmicos.
3. **RAG não é personalidade.** RAG recupera evidências relevantes para compor contexto; a identidade do agente e seus invariantes ficam estruturados e versionados.
4. **Conhecimento é local.** Nenhum agente recebe fatos que não poderia saber pela história da simulação.
5. **Exames são executáveis.** Regras, deadlines, pontuação, distribuição de papéis e condições de vitória devem ser determinísticas e testáveis.
6. **Tempo é lógico/event-driven.** 24 horas simuladas não equivalem a 24 horas reais; o relógio avança por eventos e cenas relevantes.
7. **O primeiro ano é o escopo inicial.** Ao final, fazemos snapshot/pause antes de introduzir o elenco do segundo ano.
8. **A simulação pode divergir do cânone.** O cânone fornece pessoas, regras e antecedentes; não força resultados que já deixaram de ser causalmente possíveis.

## Estado atual (apurado em 2026-11-12; `main@78bfd267`)

**Fundação causal relevante integrada; First Real Run ainda NÃO executável.** A `main` contém CSFRV1-1–8 (incluindo replay/snapshot/resume/fork da PR #25), as melhorias de autoridade, bootstrap e snapshot das PRs #61–#71 e o contrato de checkpoint cognitivo S0 da [PR #72](https://github.com/carloshgalves/cote-ai-simulation/pull/72). S0 possui interface/gate `internal/csf/ownercheckpoint/` e testes com owner injetado, **não** um serviço de memória real nem prova de durabilidade remota; a [issue #58](https://github.com/carloshgalves/cote-ai-simulation/issues/58) segue aberta como gate P0.

O HEAD da PR #72 passou nos cinco workflows pertinentes. A `main@78bfd267` passou no [workflow de conformidade V1](https://github.com/carloshgalves/cote-ai-simulation/actions/runs/43226400594). A igualdade Go/TypeScript dos 621 casos **só atesta o bundle V1**: a cobertura de CI para V2 continua pendente na [#59](https://github.com/carloshgalves/cote-ai-simulation/issues/59). Esses checks não substituem auditoria de release ou teste E2E do produto.

Canon RAG V0 e dados consolidados de referência até Y1 V11 estão integrados com barreiras de admissão; presença no corpus não torna evidências `verified:true` nem as transforma em memória vivida, regras executáveis ou objetivos obrigatórios. Auditorias #38/#39/#52 foram encerradas administrativamente, não homologadas. Embodiment PSV1-1/2 está integrado; PSV1-3–8, Observatory (CSFRV1-9), adapter físico (9A/10), economia, Examination Engine e cognição multiagente permanecem pendentes.

**Status e próxima fatia:** [auditoria e priorização de 12/11](docs/validation/project-state-2026-11-12.md). **Gates do primeiro run:** [matriz de prontidão](docs/validation/product-readiness-2026-11-10.md). O próximo trabalho recomendado é a [S1 da #58 (issue #74)](https://github.com/carloshgalves/cote-ai-simulation/issues/74), com planejamento de owner concreto e validação recuperável sem acesso a LLM real; em paralelo, a #59 é independente. Não há autorização automática de merge.

O primeiro código do repositório é o subdomínio `Embodiment` do motor, em `src/embodiment/` — amostragem de capacidade física e estado corporal que persiste no tempo, sem nenhuma chamada de LLM. **Python vale para esse subdomínio e só para ele** ([ADR 0007](docs/adr/0007-python-para-o-subdominio-embodiment.md)); framework de agentes, provedor de LLM, banco vetorial e UI seguem em aberto.

```bash
uv sync --extra dev                                                   # ou: pip install -e '.[dev]'
python -m embodiment seed-cohort --world-seed 42 --n 40 --out runs/demo/
python -m embodiment advance-clock --run runs/demo/ --character npc.0017 \
    --days 3 --sleep 4h --quality poor
pytest
```

O segundo comando avança três dias com sono ruim e mostra, por canal, o antes e o depois: `coordination` e `reaction_time` caem visivelmente, `max_strength` quase não se move, e a dívida de sono continua no corpo no dia seguinte. Rodá-lo com `--days 0` não muda nada — cena não cura.

Veja `CONTEXT.md`, `docs/adr/` e `docs/research/ai-landscape.md`.

## Causal Simulation Foundation

O contrato canônico da CSF V1 possui runners independentes em Go e TypeScript. Ambos validam o
bundle normativo e precisam emitir o mesmo relatório de 621 casos:

```bash
go run ./cmd/csf-harness --report /tmp/csf-go-report.json
node --no-warnings --experimental-loader ./tools/csf-conformance-ts/loader.mjs \
  tools/csf-conformance-ts/runner.ts docs/architecture/canonical-codec-v1-bundle \
  /tmp/csf-ts-report.json
cmp /tmp/csf-go-report.json /tmp/csf-ts-report.json
```

Para criar um run vazio, forneça um envelope CBOR canônico de genesis e um arquivo SQLite novo:

```bash
go run ./cmd/csf-harness --genesis /path/to/genesis.cbor --db /path/to/run.db
```
