# PSV1-3 — Estimador contra verdade sintética

**Spec:** §3.1 (`CAPACITY_ESTIMATOR`) · §11.3 · §12 · §14.2
**Modelo:** §5 (estimador), §6 (feats como restrições), §10 (um estimador, dois clientes)
**Bloqueado por:** PSV1-1
**Bloqueia:** — (paralelo ao PSV1-2)

## Resultado observável

```
python -m embodiment estimate --feats tests/fixtures/synthetic/cohort_a/ \
    --character synth_003 --world-seed 42
```

converte restrições tipadas em `capacity_posterior`, e imprime por dimensão: intervalo de
credibilidade, `evidence_sufficiency` e ESS. Com apenas `LOWER_BOUND`, a cauda superior sai **larga** —
e isso é o resultado correto, não uma falha de convergência.

E o teste central do ticket, que é uma asserção sobre o que o estimador **não** faz: um personagem
forte que rendeu a 60% e um mediano que rendeu a 100% produzem a mesma observação, e o estimador
**não os separa**. Se separar, está usando informação que ninguém dentro do mundo tem.

## Escopo

- **`constraints.py`.** Os seis tipos do modelo §6 e sua contribuição para o peso da partícula:
  `LOWER_BOUND` (indicadora suavizada, com margem para erro de desnormalização), `UPPER_BOUND`
  (indicadora oposta, só com atestação maximal), `INSTRUMENTED_ESTIMATE` (gaussiana com desvio =
  erro de instrumento **mais** termo de ocultação possível), `COMPARATIVE` (logística sobre a
  *diferença de desempenho no mesmo evento*, nunca diretamente sobre capacidade, exigindo
  `effort_assumption`), `TESTIMONY` (peso 1 — não entra), `NO_INFORMATION` (peso 1, por construção).
- **`estimator.py`.** Amostragem por importância com reamostragem sobre substream nomeado. Reporta
  ESS e `evidence_sufficiency` por dimensão, com a definição operacional do modelo §5:
  `1 − sd(posterior) / sd(prior)`.
- **Conduta sob ESS baixo** conforme a decisão 13.3.
- **Revalidação de atestação (F9).** O schema de feat já recusa `UPPER_BOUND` sem atestação maximal;
  o estimador **recusa de novo**. Duas guardas para a mesma regra é deliberado: é a regra que separa
  este modelo de um que mede capacidade a partir de desempenho.
- **`STRAIN_CUES_PRESENT` não qualifica para teto.** É valor separado. O análogo real de "ofegar,
  cambalear, dar tudo" é satisfeito a 61% do VO₂max; só `VERIFICATION_BOUT` — um segundo desempenho
  em condições nas quais reter esforço não era viável — atesta máximo estruturalmente.
- **Recusa de desnormalizar 持久走** enquanto S3 estiver aberta; preferir a coluna do vaivém
  (spec §12).
- **Integração com o seeding.** `seeding.py` passa a chamar o estimador quando houver restrições, sem
  mudar a assinatura entregue no PSV1-1. Personagem sem restrição continua amostrando do prior, que é
  o mesmo caminho com zero evidência — não existem dois sistemas.
- **Harness de verdade sintética** (§11.3): amostra uma coorte do prior com um seed (essa é a
  verdade), simula desempenhos com esforço conhecido, gera feats tipados, roda o estimador **sem** ver
  a verdade.
- **`estimator-params.yaml`.** Nº de partículas, largura das indicadoras suavizadas, erro de
  instrumento, termo de ocultação, limiar de ESS. Tudo `[INT]`, declarado.

## Arquivos e módulos

```
src/embodiment/{constraints,estimator,seeding,cli}.py
data/models/physical/estimator-params.yaml       novo
tests/fixtures/synthetic/                        novo — coorte sintética com verdade conhecida
tests/embodiment/{test_constraints,test_estimator,test_estimator_recovery}.py
tests/embodiment/properties/test_p4_floor_is_not_ceiling.py
tests/embodiment/invariants/test_invariant_{02,10,12}_*.py
tests/embodiment/scenarios/test_scenario_{02,06,08}_*.py
```

## Decisões que consome

13.3 — limiar de ESS e conduta abaixo dele.

## Testes determinísticos

### Recuperação contra verdade sintética (spec §11.3 — os quatro)

1. **Cobertura.** O intervalo de credibilidade contém a verdade na frequência nominal.
2. **Assimetria.** Só com `LOWER_BOUND`, a cauda superior permanece larga; o posterior **não**
   converge para a verdade. Sob identificação parcial, convergir seria o bug.
3. **Ocultador × mediano.** Um forte a 60% e um mediano a 100% produzem a mesma observação e o
   estimador não os separa sem atestação de esforço. Este é o teste que diz se o modelo está certo.
4. **Valor do teto.** Acrescentar um `VERIFICATION_BOUT` estreita a cauda superior de forma
   mensurável — e é a **única** coisa que a estreita.

### Demais

- **Encolhimento hierárquico sem maquinaria extra.** Com pouca evidência, o posterior fica próximo da
  coorte condicionada; é comportamento padrão da amostragem por importância, e o teste verifica que
  ninguém acrescentou um mecanismo separado para produzi-lo.
- **ESS e degeneração (F8).** Sob restrições apertadas, o ESS cai e a conduta da decisão 13.3 dispara.
  Um posterior colapsado em uma partícula **não** é devolvido silenciosamente.
- **`UPPER_BOUND` sem atestação é recusado (F9)**, com erro que nomeia o feat.
- **`TESTIMONY` não move o posterior.** Peso exatamente 1 para toda partícula; a asserção é sobre os
  pesos, não sobre o resultado — um posterior que por acaso não mudou não prova nada.
- **`COMPARATIVE` sem `effort_assumption` é recusada**, e com ele opera sobre diferença de desempenho
  no mesmo evento, nunca sobre capacidade (invariante 8).
- **Auditabilidade partícula a partícula.** Com o mesmo substream, reexecutar e inspecionar por que
  uma restrição empurrou o posterior. O teste reexecuta e compara o traço.
- **Guarda de repositório (AC 11).** Nenhum `capacity_posterior` de personagem canônico é escrito em
  `data/`; a saída do estimador para personagem canônico só existe em memória durante o seeding.

## Property tests

- **P4 — piso nunca vira teto.** Para qualquer conjunto de restrições **sem atestação maximal**, o
  posterior mantém massa acima do maior piso observado. Pisos são baratos, tetos são caros, e essa
  assimetria precisa valer para *qualquer* conjunto de entradas, não para os que escolhemos testar.

## Cenários

- **2 — o vencedor que desiste por tédio.** A parcial percorrida entra como `LOWER_BOUND`; a dimensão
  aeróbica recebe `NO_INFORMATION`. O abandono é evento de disposição e pertence a outro dono — o
  teste verifica que o estimador não o interpreta como resistência baixa.
- **6 — feat posterior à divergência.** Informa o posterior no seeding, porque o corpo é o mesmo
  corpo. Depois do seeding não informa mais nada; a metade "ausente de memória e crença" é asserida no
  PSV1-6.
- **8 — testemunho contra medida.** "Ele é o mais forte do ano" entra como `TESTIMONY`, vai para o
  belief do `testifier` e não para o posterior. Se entrasse, hype canônico viraria física.

## Checagens de fronteira de conhecimento

- O `capacity_posterior` numérico **não é documento recuperável por RAG** (classe de vazamento 4 da
  spec §7.3). O corpus de feats é recuperável em paráfrase; o posterior é insumo de seeding. O teste
  verifica que nenhuma saída do estimador é escrita em um caminho indexável.
- ESS, pesos de partícula e substreams são telemetria de engine: vivem no event log e nos metadados do
  run, nunca em payload de contexto.

## Fora do escopo

Ingestão de feats canônicos reais — `data/canon/feats/` continua vazio, bloqueado por
`oq.capability.*`, e permanece assim durante toda a V1. O cliente B do modelo §10 (um agente
inferindo sobre outro, incrementalmente) é o PSV1-5, que compartilha a semântica de restrição mas não
o custo computacional.

## Evidência de conclusão

1. O comando da seção de resultado roda sobre a fixture sintética e imprime intervalo, sufficiency e
   ESS por dimensão.
2. Os quatro testes de recuperação passam, e o terceiro — ocultador × mediano — está nomeado de forma
   que uma falha aponte para o cenário 1 do modelo §14.
3. P4 verde sob `hypothesis`.
4. `data/canon/feats/` continua vazio, e nenhum posterior de personagem canônico aparece no diff.
5. `estimator-params.yaml` passa no validador de cabeçalho do PSV1-1, com todos os parâmetros
   marcados `[INT]`.
