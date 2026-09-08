# ADR 0003 — Tempo lógico/event-driven, não tempo real

**Status:** Accepted

## Contexto

Vincular 24 horas simuladas a 24 horas reais tornaria exames de vários dias lentos e caros, além de obrigar chamadas de modelo em períodos sem relevância.

## Decisão

Usar um relógio lógico com fila de eventos. O engine pode:

- avançar até o próximo evento relevante;
- executar cenas detalhadas em pequenos passos;
- fast-forward de períodos sem decisão relevante;
- pausar manualmente;
- salvar/restaurar snapshots.

A duração de uma ação altera o horário simulado, mas não define quanto tempo de parede a execução precisa levar.

## Consequências

Um exame de uma semana pode ser processado em minutos ou horas de computação, dependendo da granularidade e quantidade de chamadas, sem perder a cronologia interna.
