# ADR 0007 — Python para o subdomínio `Embodiment`, e só para ele

**Status:** Accepted
**Data:** 2026-09-09
**Relacionado:** ADR 0006 (domínio físico), ADR 0003 (tempo lógico), ADR 0005 (snapshot canônico compilado)
**Origem:** decisão aberta 13.4 da [spec Physical Simulation V1](../spec/physical-simulation-v1.md), fechada pelo ticket [PSV1-0](../tickets/physical-simulation-v1/00-decisoes-abertas.md)

## Contexto

O `README.md` afirma que o projeto ainda não escolheu framework, provedor de LLM, banco vetorial nem
stack de UI, e o `CONTEXT.md` coloca "escolher framework definitivo antes de um prototype
comparativo" fora de escopo. A spec do Physical Simulation V1, porém, precisa de uma linguagem para
escrever `src/embodiment/` — e escolhê-la de dentro da spec faria a escolha valer para o projeto
inteiro por omissão. Ninguém decidiu que o Agent Cognition é Python; mas se o primeiro código do
repositório for Python e nada disser o contrário, isso terá sido decidido na prática, sem ADR e sem
alternativa avaliada.

Este subdomínio é atipicamente independente do resto. Ele faz amostragem multivariada (cópula
gaussiana sobre marginais, amostragem por importância com reamostragem), funções puras de dinâmica
corporal e resolução determinística de esforço e disputa. Não faz nenhuma chamada de LLM — a spec §4
classifica **toda** a V1 como determinística —, não tem serviço, não tem UI, e sua única I/O é ler
YAML de parâmetros e escrever JSONL e snapshot. As ferramentas que a spec §5.2 exige (`numpy`,
`pydantic`, `pytest`, `hypothesis`) são exatamente a região onde o ecossistema Python é mais forte, e
nenhuma delas é uma dependência de agentes, de RAG ou de orquestração.

Ou seja: a decisão que a V1 realmente precisa tomar é pequena, e a decisão que ela **não** deve tomar
por acidente é grande.

## Decisão

### 1. `src/embodiment/` é Python

Toolchain: `numpy` para cópula e SIR, `pydantic` para tipos de estado, `pytest` + `hypothesis` para
os testes, `pyproject.toml` na raiz (introduzido pelo PSV1-1). Sem framework de aplicação: a V1 não
tem serviço.

### 2. O escopo é o subdomínio, não o projeto

Esta decisão **cobre** `src/embodiment/`, sua toolchain, seus testes e os arquivos de parâmetro em
`data/models/physical/`.

Esta decisão **não cobre**, e não deve ser citada como precedente para: Agent Cognition, Examination
Engine, Social State, Canon Knowledge, Year Transition, framework de agentes, orquestração
multiagente, provedor de LLM, banco vetorial, camada de persistência do mundo, ou UI. Cada um desses
continua em aberto exatamente como o `README.md` afirma.

### 3. A fronteira do subdomínio é dados, não importação de módulo

Nada fora do `Embodiment` depende de importar um módulo Python dele. O que atravessa a fronteira é:

- o **event log** append-only (JSONL), incluindo `capacity.sampled`, `exertion.resolved`,
  `observation.emitted` e os demais eventos da spec §5.4;
- o **snapshot** serializado com hash (spec §5.3);
- os **arquivos de parâmetro** versionados em `data/models/physical/`.

Os três já existem por outras razões — ADR 0003 (tempo lógico e snapshot) e ADR 0005 (snapshot
compilado com hash) —, então a fronteira não precisa ser inventada depois para sustentar esta
decisão. Ela já é a forma como o engine conversa consigo mesmo.

### 4. Stack diferente adiante é fronteira de processo, não erro

Se o Agent Cognition ou o Examination Engine forem para outra linguagem, o `Embodiment` passa a ser
consumido por fronteira de processo — subprocesso, ou serviço local, sobre os mesmos artefatos de
dados do item 3. O custo previsto é serialização e latência de fronteira. **Não** é reescrita, e não
é retratação deste ADR: é o cenário que ele antecipa.

### 5. O que reabriria esta decisão

Duas condições, nenhuma delas conhecível hoje:

1. o `Embodiment` precisar ser chamado de forma síncrona de dentro do laço de decisão do Agent
   Cognition, sob orçamento de latência que a fronteira de processo não sustente;
2. a stack do engine ser escolhida em outra linguagem **e** o custo acumulado da fronteira superar o
   custo de reescrever um pacote de funções puras sem dependência de framework.

Qualquer uma delas exige ADR próprio. Generalizar esta escolha para outro subdomínio também exige ADR
próprio: usar este documento para afirmar "o projeto é Python" é uso indevido dele.

## Alternativas descartadas

**Adiar a escolha até o protótipo comparativo de framework.** Bloquearia a V1 inteira por uma decisão
que ela não consome. O `Embodiment` é o candidato de menor acoplamento do repositório justamente
porque não usa LLM nenhum; deixá-lo esperando pela escolha de stack de agentes seria acoplar por
processo o que o domínio desacoplou.

**Decidir a stack do projeto inteiro agora.** Escolheria framework de agentes, provedor e banco
vetorial a partir das necessidades de uma amostragem numérica — que são as menos representativas do
que o projeto realmente vai exigir. É a ordem errada, e o `CONTEXT.md` já a coloca fora de escopo.

**TypeScript para tudo desde já.** Defensável para o Agent Cognition e para uma futura UI, fraco
exatamente onde este subdomínio vive: cópula gaussiana, SIR com dezenas de milhares de partículas,
property tests com `hypothesis`. Trocaria a fronteira de processo (item 4) por reimplementação
numérica, que é o custo maior dos dois.

## Consequências

- A V1 pode começar. O PSV1-1 abre `pyproject.toml` sem reabrir esta questão, e nenhum ticket
  seguinte precisa tomar a decisão de dentro de um critério de aceitação.
- O repositório passa a ter uma linguagem no `src/`, e a afirmação do `README.md` continua verdadeira:
  framework, provedor de LLM, banco vetorial e UI seguem sem escolha.
- Toda dependência acrescentada ao `pyproject.toml` é dependência **do subdomínio**. Uma dependência
  que só faça sentido para agentes, RAG ou UI entrando por aqui é sinal de que a fronteira do item 3
  está sendo furada.
- O teste de arquitetura do PSV1-1 (`src/embodiment/` não importa cliente de LLM, não usa RNG global)
  é também o guarda desta fronteira, não só do determinismo.
