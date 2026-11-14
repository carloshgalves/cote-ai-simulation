# CSFRV1-9A/A3 — Matriz exaustiva de disclosure físico

**Issue #100 · Status: contrato de classificação/fixtures, não agente perceptivo nem admissão causal.**

## Autoridade

A fonte A2 `physical.verified-transition-evidence.v1` é exclusivamente engine-only. A conformidade do schema não converte hashes, `BodyState` ou `body.advanced` em `Observation`, `KnowledgeInput` ou `ActorContext`. O Go ainda precisa autenticar origem e destinatário, derivar a percepção a partir de fonte física efetivamente verificada, registrar a policy no Genesis e emitir um registro causal pela transação oficial. Até então, a barreira é **DENY**.

O arquivo [physical-disclosure-matrix-a3.json](physical-disclosure-matrix-a3.json) classifica **cada caminho terminal do schema A2** em uma categoria, sem herança permissiva:

| Categoria | Regra |
|---|---|
| `ENGINE_ONLY` | Referência/proveniência interna, jamais entregue diretamente a agentes |
| `FORBIDDEN_IN_CONTEXT` | Segredos, estado corporal bruto, identificadores/digests sensíveis e correlatos; nunca são evidence/context |
| `QUALITATIVE_DERIVATION_ONLY` | Pode alimentar *futura* percepção qualitativa restrita ao sujeito/observador autenticado, nunca os valores brutos |

Todas as categorias têm `direct_disclosure=false`. Nenhum campo de corpo possui classificação por omissão. A3 não implementa conversão fisiológica numérica para rótulos qualitativos: os rótulos de exemplo são apenas **formato de candidato não admitido**, sem prova de causalidade/percepção.

## Política e invariantes executáveis

- `tests/architecture/embodiment_a3_disclosure.py` percorre `$ref`, `oneOf`, `object` e `array` do schema A2. Exige que todos os objetos sejam fechados, que todos os campos sejam obrigatórios e que **o conjunto exato de caminhos** coincida com a matriz versionada. Um campo novo, um `$ref` desconhecido ou um ramo aninhado novo provoca falha fechada.
- O formato de proposta qualitativa tem conjunto exato de oito campos, labels enumerados, `authority=NOT_ADMITTED`, `subject_id=observer_id=recipient_id`, e hash de proveniência do ator. O formato não aceita corpo, posterior, RNG, EventOrderKey, WorldRevision, texto arbitrário ou campos adicionais.
- O parser referência nunca devolve um registro `Observation` ou `KnowledgeInput`. A rotina `refuse_agent_context` rejeita tanto corpos brutos quanto candidatos. O `contextboundary.Build` de Go permanece com allow-list própria, explicitamente sem domínio físico.
- Os hashes de A2 são indicadores de consistência relativa a bytes, **não autenticam o executor**. IDs fornecidos pelo próprio candidato **não autenticam** o sujeito ou a fonte: Go, via A1/A4, deve comprovar isso separadamente.
- Erros de recusa usam uma mensagem fixa, não incluem números de BodyState, nomes/IDs não confiáveis, valores de source ou documentos privados.

## Corpus de ataques

A [suíte A3](../../../tests/architecture/test_embodiment_a3_disclosure.py) constrói runs reais de **dois personagens** com `seed_cohort_command` e `advance_clock_command`, projeta via A2 PSV1-2 e testa: unknown field, drift de versão, ramo adicional, hash inválido, unidade fora do domínio, RNG/posterior extra, corpo/telemetria de terceiro, observador ou destinatário errado, forja de GO authority, event/world IDs e ausência de sujeito, payload bruto/arbitrário e erros que exponham segredos. Confere os arquivos originais imutados.

## Limites e dependências

1. **NÃO** é autorização de leitura de corpo por atores; os candidatos positivos são *shape-only*, `NOT_ADMITTED`, sem distribuição.
2. **NÃO** é integração com `CommitCoordinator` ou percepções reais. A3 classifica o schema A2 presente e recusa qualquer novo campo sem política explícita e revisão.
3. A1 deve autenticar subprocesso, source cut e Genesis; A4 deve testar a integração/canonicalização; a futura CSFRV1-10 precisa implementar derivação, observador, autorização e registro de `Observation` sob política Go.
4. PSV1-8 completa não é pré-requisito para classificar A2 PSV1-2, mas novos estados físicos exigem revisão e expansão explícitas da matriz.

## Gates

- [ ] CI A3/schema A2, evidências reais e testes negativos no SHA final.
- [ ] Revisão adversarial com registro de findings, correção e re-review sem novos findings locais acionáveis.
- [ ] Revisão independente por revisor distinto e merge humano.
- [ ] A4 #101 e admissão verdadeira #108 seguem abertas; First Real Run **NO-GO**.
