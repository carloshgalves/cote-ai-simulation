# PSV1-7 — Resolvedor de disputa por trocas

**Spec:** §3.1 (`CONTEST_RESOLVER`) · §6 (ordem por troca) · §11.6 · §14.5
**Modelo:** §11 (combate)
**Bloqueado por:** PSV1-5
**Bloqueia:** PSV1-8

## Resultado observável

```
python -m embodiment run-contest --run runs/demo/ --modality restrain \
    --a npc_017 --a-objective hold_back --b npc_004 --b-objective subdue \
    --observers npc_019 --world-seed 42
```

resolve um confronto de contenção em 3 a 10 trocas e devolve: quem controlou quem, quanto custou, o
que os observadores viram, e o motivo da terminação. A lesão resultante **persiste para o evento
seguinte** — rodar `advance-clock` depois mostra o corpo ainda comprometido.

E o event log mostra, por troca, os termos da margem **separadamente**: habilidade, capacidade, massa,
posicional, penalidade, ruído. Uma luta cujo resultado não se explique pelos termos registrados é bug
de instrumentação, não de balanceamento.

## Escopo

- **`contest.py` — máquina de 3 a 10 trocas.** Por troca, nesta ordem: margem → logística → resultado
  → consumo de reserva → sorteio de lesão → observação → teste de terminação.
- **A logística é aplicada por troca, não por luta.** Esta é a escolha estrutural do ticket. Aplicar
  por luta apaga, sem aviso, as duas propriedades que o resolvedor precisa ter — e as apaga de um jeito
  que nenhum teste de resultado individual detecta.
- **A margem**, com os termos do modelo §11:
  ```
  margem = w_hab(Δcapacidade) · Δhabilidade
         + w_cap · Δcapacidade_relevante_à_modalidade
         + w_massa · Δbody_mass
         + termo_posicional (controle, distância, iniciativa)
         − penalidade(fadiga central, dor)      ← degrada habilidade mais que força
         + ruído(substream nomeado)
  ```
  A função de ligação é Bradley-Terry/Elo: o formalismo padrão para mapear a *diferença* entre forças
  latentes em probabilidade de vitória. `body_mass` é termo explícito porque categorias de peso
  existem por um motivo.
- **Estado mínimo entre trocas:** controle/posição, distância, iniciativa, reserva por participante
  (`w_prime_balance`), dor, dano acumulado e satisfação de objetivo.
- **Terminação** por objetivo atingido, reserva esgotada, lesão incapacitante ou intervenção de
  terceiro.
- **Objetivos assimétricos mudam a função objetivo.** Escapar não é vencer, conter não é ferir. Um
  participante com `hold_back` ou `lose_deliberately` usa o mesmo `display_ceiling` do PSV1-4.
- **Toda saída escreve em `BodyState`** pela API única do PSV1-2, como qualquer outro esforço.
- **Observação por troca**, reusando o PSV1-5: é daí que sai "lutar revela", como consequência e não
  como postulado.
- **`contest-params.yaml`.** Escala da logística por troca, forma de `w_hab`, `w_cap`, `w_massa`,
  penalidades de fadiga e dor, faixa de trocas. Tudo `[INT]`: parametrização declarada, **calibrada
  por propriedade e não por dado**, e o arquivo diz isso.
- **Esqueleto versionado da família `simulation-evals`** (spec §11.6): comparação entre runs sobre ≥ 30
  seeds, não história única.

## Arquivos e módulos

```
src/embodiment/{types,contest,cli}.py
data/models/physical/contest-params.yaml         novo
evals/simulation-evals/                          novo — esqueleto versionado
tests/embodiment/{test_contest_exchange,test_contest_termination,test_contest_objectives}.py
tests/embodiment/properties/test_contest_emergent_properties.py
tests/embodiment/invariants/test_invariant_{10,13}_*.py
```

## Decisões que consome

13.1 — forma funcional de `w_hab(Δcapacidade)`. O ticket implementa a forma escolhida e registra a
calibração **por propriedade** como tal em `contest-params.yaml`. As duas propriedades de calibração
são as da própria decisão: "um judoca leve vence um aluno forte e destreinado" e "uma diferença grande
de capacidade não é compensável por técnica".

## Testes determinísticos

- **As duas propriedades emergentes são medidas, não assumidas** (AC 5). Sobre muitos seeds:
  - **disputas longas favorecem condicionamento**, porque cada troca consome reserva e a degradação
    composta domina;
  - **disputas curtas favorecem técnica e iniciativa**, porque poucas trocas não dão tempo de a reserva
    importar.
  O teste compara distribuições de vencedor entre pares construídos para isolar cada eixo. Uma
  propriedade "derivada" que ninguém mediu é uma propriedade afirmada.
- **`w_hab` é decrescente em `Δcapacidade`**, monotonicamente, em todo o domínio. Habilidade domina em
  diferenças pequenas, capacidade em diferenças grandes — como função, não como regra à parte.
- **Faixa de trocas.** Nenhuma disputa termina com menos de 3 nem mais de 10 trocas.
- **Terminação por cada um dos quatro motivos** é alcançável, com uma fixture por motivo.
- **Objetivos assimétricos.** `escape` satisfeito não implica `subdue` frustrado da mesma forma que
  uma vitória simétrica; `hold_back` produz saída diferente de `all_out` com os mesmos corpos.
- **Preensão e `strength_endurance` são de primeira classe em agarre.** Na modalidade `grapple`, a fase
  de pegada consome cerca de metade do tempo de combate e a resistência de preensão é determinante
  para projeções e imobilizações; o teste falha se essas dimensões não pesarem na margem da
  modalidade.
- **Condicionamento não é tudo.** Ele responde por **até 45%** da variância entre lutadores bem e mal
  sucedidos — grande, mas longe de tudo. O teste verifica que habilidade e estado carregam o resto, e
  que nenhum termo isolado domina a margem.
- **O corpo não se resolve no fim da cena.** Depois do contest, `BodyState` mostra reserva consumida,
  dor e eventual lesão; um `advance-clock` seguinte parte desse estado (invariante 4).
- **Consequências institucionais não vivem aqui.** Teste de arquitetura: `contest.py` não emite
  advertência, dedução de ponto nem expulsão. O combate emite eventos; a escola os julga.
- **Nada de granularidade proibida.** Nenhum campo de localização de golpe, alavanca articular, física
  de impacto ou ordem de iniciativa por décimos de segundo.

## Evals

**Família `simulation-evals`, esqueleto versionado.** Sobre ≥ 30 seeds, duas verificações que só fazem
sentido entre runs:

1. onde a evidência não separa dois personagens, os **vencedores variam**;
2. a suspeita de ocultação cresce com o **número** de observações, não com um único evento — **exceto
   após um combate**, onde muitas observações correlacionadas são emitidas de uma vez.

A segunda existe porque "lutar revela" é **derivada** no modelo §11, e uma propriedade derivada
precisa ser medida. Para um ocultador, esse é o custo dominante de lutar — e o eval é o que confirma
que o custo existe no código, e não só no texto.

Nenhuma das duas depende de LLM. A família fica versionada e sem casos de modelo.

## Checagens de fronteira de conhecimento

- **A observação por troca passa pelo PSV1-5**, com o mesmo erro e o mesmo teto de log-odds. O contest
  não tem canal de observação privilegiado.
- **A atualização de crença é amortecida quando a incerteza sobre o adversário é alta** — é o mesmo
  comportamento de Glicko do PSV1-5, e o teste verifica que o contest não o contorna emitindo
  observações "melhores".
- **Nenhum participante lê o `BodyState`, a reserva ou o `ExertionIntent` do outro.** Um teste de
  arquitetura recusa esse caminho dentro de `contest.py`, que é onde ele seria mais tentador.

## Fora do escopo

Consequências institucionais, disciplinares e sociais. Simulação biomecânica golpe a golpe.
Localização de dano. Integração com `ExamSpec`.

## Evidência de conclusão

1. O comando da seção de resultado roda e devolve trocas, motivo de terminação e satisfação de
   objetivo por participante.
2. `contest.exchange` registra os **seis termos da margem separadamente**, mais resultado, custos e
   observações emitidas; `contest.ended` registra motivo e satisfação.
3. Um `advance-clock` posterior parte do corpo alterado pela disputa.
4. As duas propriedades emergentes medidas, com o resultado da medição registrado no ticket — não
   apenas um teste verde.
5. `contest-params.yaml` declara todos os parâmetros como `[INT]` e registra a calibração por
   propriedade como calibração por propriedade.
6. `evals/simulation-evals/` existe, versionada, com as duas verificações descritas e um README que
   diz por que elas são comparações entre runs.
