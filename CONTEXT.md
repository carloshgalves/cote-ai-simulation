# Domain Context — COTE AI Simulation

## Objetivo

Construir uma simulação multiagente em que personagens inspirados em *Classroom of the Elite* enfrentem exames especiais existentes sob regras formais, mantendo personalidade, memória, conhecimento parcial e relações persistentes. A simulação deve permitir resultados alternativos sem um roteirista corrigindo o mundo para reproduzir o cânone.

## Bounded contexts

### Simulation Engine
Fonte de verdade sobre relógio, localização, recursos, eventos, causalidade, turnos/cenas e snapshots.

**Subdomínio `Embodiment`.** Capacidade física amostrada, estado corporal (fadiga aguda e acumulada,
sono, nutrição, hidratação, carga térmica, dor, lesão), ambiente, resolução de esforço e resolução de
disputa física. É subdomínio do engine, não contexto par: um módulo físico ao lado do engine criaria
dois lugares capazes de decidir o que aconteceu. Modelo em
[`docs/architecture/physical-model.md`](docs/architecture/physical-model.md), decisão no
[ADR 0006](docs/adr/0006-physical-domain-model.md).

### Agent Cognition
Transforma percepção + identidade + memória + crenças + objetivos em intenção/ação. Nunca recebe o world state completo.

### Canon Knowledge
Armazena perfis estruturados e evidências comportamentais usadas para fidelidade de personagem. Não contém o estado corrente da simulação. Inclui o corpus **compartilhado** de feats físicos (`data/canon/feats/`), indexado por ator e não separado por personagem.

### Agent Knowledge & Memory
Mantém o que cada agente sabe, acredita, suspeita, esquece, aprendeu e de onde veio a informação.

### Examination Engine
Modela exames especiais como especificações executáveis: participantes, regras, papéis secretos, deadlines, canais de informação, ações válidas e scoring.

### Social State
Relações, reputação, confiança, dívidas, alianças, rivalidades e compromissos. É consequência da simulação, não do cânone depois do instante inicial.

### Year Transition
Congela um snapshot ao fim do primeiro ano e injeta novos entrantes a partir de perfis pré-chegada, reconciliando-os com a linha do tempo divergente.

## Invariantes

1. World truth e agent belief são estruturas distintas.
2. Toda informação disponível a um agente deve ter proveniência.
3. Inteligência alta não concede acesso a fatos ocultos; melhora inferência sobre evidências disponíveis.
4. Regras de exame são resolvidas deterministicamente pelo engine.
5. Ações impossíveis ou inválidas são rejeitadas pelo engine, não reinterpretadas como sucesso pelo LLM.
6. Memórias da linha canônica após o ponto de divergência não podem aparecer como memória da simulação.
7. Mudanças de prompt/modelo que alterem comportamento estratégico exigem evals versionados.
8. Corpus privado não deve ser commitado no repositório público.
9. Capacidade latente, estado corporal, esforço escolhido, desempenho realizado, desempenho observado e autopercepção corporal são seis estruturas distintas.
10. Desempenho observado restringe capacidade como limite, nunca como estimativa; teto só existe com esforço máximo atestado.
11. Ausência de evidência sobre um personagem produz prior populacional versionado, nunca atributo escolhido à mão.
12. Consequência física persiste entre cenas, eventos e dias; cena não cura nada, e só o engine escreve estado corporal.
13. Ao agente chega interocepção qualitativa e enviesada, nunca telemetria corporal — nem a própria, nem a de terceiros.
14. Comparações de capacidade entre personagens são saída da simulação, nunca entrada de dados.

## Escopo da primeira milestone

- Primeiro ano apenas.
- Poucos agentes focais de alta fidelidade e NPCs simplificados para o restante da escola.
- Um exame especial completo como vertical slice.
- Relógio lógico com pause, step e fast-forward.
- Snapshot reprodutível por seed/configuração.
- Evals de personagem, RAG, informação e resultado de exame.

## Fora de escopo por enquanto

- Reproduzir 300 alunos como 300 processos/LLMs permanentes.
- Segundo ano rodando automaticamente antes de validar o primeiro ano.
- UI 3D.
- Treinar/fine-tunar um modelo próprio antes de provar que prompting + memória + retrieval são insuficientes.
- Escolher framework definitivo antes de um prototype comparativo.
