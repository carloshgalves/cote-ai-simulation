# ADR 0004 — Fechar o primeiro ano antes de introduzir o segundo

**Status:** Accepted

## Contexto

Personagens do segundo ano, incluindo alunos ligados ao White Room, entram em um mundo cujo primeiro ano pode ter divergido fortemente do cânone.

## Decisão

A primeira versão termina em um **pause gate** no encerramento do primeiro ano.

Nesse ponto:

1. persistimos snapshot completo do mundo;
2. avaliamos estabilidade/fidelidade da execução;
3. criamos perfis pré-chegada dos novos alunos;
4. reconciliamos somente o conhecimento que eles poderiam possuir sobre a escola atual;
5. introduzimos o elenco do segundo ano como uma nova fase/migração de mundo.

Antecedentes anteriores à chegada podem permanecer canônicos. Eventos escolares que não ocorreram nesta simulação não podem ser importados como memória factual dos novos agentes.

## Consequências

O segundo ano não bloqueia a arquitetura inicial e pode ser desenvolvido depois sem reescrever a simulação do primeiro ano.
