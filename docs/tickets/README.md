# Tickets

Fatias de implementação derivadas de uma spec aceita. Um ticket é um **tracer bullet**: entrega
comportamento observável de ponta a ponta, não uma camada.

Todo ticket traz: resultado observável; escopo e não-escopo; arquivos/módulos tocados; arestas de
dependência; testes determinísticos; property tests; evals quando aplicável; checagens de fronteira
de conhecimento quando aplicável; e evidência de conclusão.

Um ticket **não** reabre a spec que o originou. Se a implementação mostrar que a spec está errada, a
spec é corrigida primeiro e o ticket é reescrito depois.

| Conjunto | Spec de origem | Estado |
|---|---|---|
| [Physical Simulation V1](physical-simulation-v1/) | [`docs/spec/physical-simulation-v1.md`](../spec/physical-simulation-v1.md) | aberto |
