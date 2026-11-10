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

## Estado atual (verificado em 2026-11-10)

**Fundação técnica real, produto não executável ponta a ponta.** A `main` integra CSFRV1-1–7 (Go/SQLite, tempo lógico, coortes, decisões, commits atômicos, evidência e comunicação), o Canon RAG V0, dados consolidados até Y1 V11 e as primeiras entregas físicas em Python. **CSFRV1-8 permanece em Draft na PR #25**; Observatory, adapter causal do Embodiment, cognição multiagente, economia executável e Examination Engine completo estão pendentes.

Os 621 vetores do codec normativo V1 são prova de conformidade do protocolo, não de simulação integrada. Os dados canônicos integrados não são automaticamente `verified:true` nem autorizados para `engine_memory_admission`. Issues #38/#39/#52 foram encerradas administrativamente em 10/11, sem certificado de revisão externa.

**Matriz de validação e critérios do First Real Run:** [`docs/validation/product-readiness-2026-11-10.md`](docs/validation/product-readiness-2026-11-10.md). Workflow manual de regressão (não libera produção): `.github/workflows/product-validation-on-demand.yml`.

Framework de cognição, provedor de LLM, banco vetorial e stack de UI seguem em aberto.

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
