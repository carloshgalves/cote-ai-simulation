# PSV1-0 — Fechar as decisões abertas da §13

**Spec:** [physical-simulation-v1.md](../../spec/physical-simulation-v1.md) §13
**Bloqueado por:** —
**Bloqueia:** PSV1-1 (13.4) · PSV1-2 (13.2) · PSV1-3 (13.3) · PSV1-7 (13.1)
**Natureza:** documental. Nenhuma linha de código.

## Resultado observável

A §13 da spec não tem mais nenhuma decisão sem uma linha `**Decidido:**` com data, escolha e
justificativa, e existe um `ADR 0007` aceito delimitando até onde vai a escolha de Python. Um
implementador que abra qualquer ticket seguinte encontra a decisão que ele consome já tomada, e não
precisa tomá-la de dentro de um critério de aceitação — que é exatamente o que a §13 existe para
impedir.

Este ticket é primeiro porque a spec o coloca primeiro: 13.1 e 13.2 são pré-requisito declarado de
tickets específicos, e 13.4 recomenda o ADR **antes** de `/to-tickets`. Fechá-las durante a
implementação faria a escolha se esconder dentro de um commit de código.

## Escopo

Quatro decisões a tomar e uma a confirmar.

### 13.1 — forma funcional de `w_hab(Δcapacidade)`

O modelo exige peso da habilidade **decrescente na diferença de capacidade** e não fixa a forma. A
escolha muda o comportamento na faixa intermediária, que é onde quase todo confronto escolar
acontece.

*Recomendação da spec:* decaimento exponencial com um único parâmetro de escala, **calibrado por
propriedade** — "um judoca leve vence um aluno forte e destreinado" e "diferença grande de capacidade
não é compensável por técnica" — e não por dado. A decisão precisa registrar a calibração por
propriedade **como tal**; um parâmetro calibrado por propriedade que se apresente como medido é a
desonestidade que o `[INT]` existe para evitar.

### 13.2 — granularidade de `peripheral_fatigue` por região

(a) manter `legs, arms, grip, core` para fadiga e permitir regiões finas só em `Injury`, aceitando e
**documentando** a assimetria entre as duas taxonomias; ou (b) unificar as duas taxonomias, mais
coerente e mais caro em toda a dinâmica.

Restrição real: o cenário 4 do modelo §14 opera na distinção punho × mão, e é um cenário de `Injury`,
não de fadiga. Isso não decide a questão, mas define o teste que a decisão precisa sobreviver.

### 13.3 — limiar de ESS e conduta abaixo dele

Falhar alto, reamostrar com mais partículas, ou devolver o posterior marcado como degenerado.
*Recomendação da spec:* falhar alto na V1 — um posterior degenerado que circula é pior que um seeding
que não completa, porque produz números plausíveis e errados. A decisão precisa fixar **o número**,
não só a conduta, porque `estimator-params.yaml` o carrega.

### 13.4 — Python fixa a stack do projeto?

Vira `ADR 0007`, com escopo **explicitamente limitado ao subdomínio `Embodiment`**. O `README.md`
afirma que o projeto ainda não escolheu framework, e esta spec escolhe Python para um subdomínio cuja
natureza — amostragem multivariada, funções puras, zero LLM — torna a escolha quase independente do
resto. O ADR registra: o que a escolha cobre, o que ela não cobre, e que se o Agent Cognition ou o
Examination Engine forem para outra linguagem, isto vira uma fronteira de processo e não um erro.

### 13.5 — `illnesses` sem dinâmica

*Recomendação da spec:* manter o campo em `snapshot_version: 1`, serializado e sem dinâmica, porque
um exame de sobrevivência vai precisar dele e o incremento de versão é mais caro que um campo vazio.
Confirmar ou reverter; se confirmar, registrar que nenhum ticket da V1 o atualiza — para que a
ausência de dinâmica não seja lida adiante como bug.

## Arquivos tocados

- `docs/adr/0007-python-para-o-subdominio-embodiment.md` — novo, status `Accepted`.
- `docs/spec/physical-simulation-v1.md` §13 — uma linha `**Decidido:**` por decisão, com data.
- `docs/tickets/physical-simulation-v1/README.md` — nada a mudar se as decisões seguirem as
  recomendações; se alguma divergir, corrigir o ticket que a consome antes de fechar este.

## Fora do escopo

Escolher a stack do Agent Cognition, do Examination Engine ou de qualquer outro subdomínio. Escrever
os arquivos de parâmetro — eles pertencem aos tickets que os usam; aqui só se fixa o que eles vão
declarar.

## Evidência de conclusão

1. `ADR 0007` existe, com status `Accepted` e escopo limitado ao `Embodiment` no próprio título.
2. `grep -c '\*\*Decidido:\*\*' docs/spec/physical-simulation-v1.md` retorna 5.
3. Nenhuma das cinco decisões aparece em qualquer critério de aceitação da §14 — a §14 continua
   pressupondo que foram tomadas, não *como* foram tomadas.
4. `git diff --stat` não toca nenhum arquivo fora de `docs/`.
