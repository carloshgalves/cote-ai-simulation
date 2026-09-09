# PSV1-6 — Interocepção e fronteira de conhecimento

**Spec:** §3.1 (`INTEROCEPTION`) · §7 integral · §11.2 (P7) · §11.6 · §14.7
**Modelo:** §9 (propriocepção), §15 invariante 6
**Bloqueado por:** PSV1-5
**Bloqueia:** PSV1-8

## Resultado observável

```
python -m embodiment show-interoception --run runs/demo/ --character npc_017
```

imprime o que `npc_017` sente: "pernas pesadas", "difícil manter foco", "punho dói ao usar" — e
**nenhum número**. Ao lado, com `--debug-truth`, o `BodyState` real, para comparação de quem lê o run
de fora. A distância entre as duas colunas é o produto do ticket: o sinal é informativo, ordinal e
enviesado, e o viés é grande mesmo no melhor caso.

E a `knowledge-boundary-audit` sobre as cinco classes de vazamento da spec §7.3 roda e não encontra
nenhuma.

## Escopo

- **`interoception.py`.** Tradução determinística de `BodyState` em sinais qualitativos enviesados,
  pelo mapa do modelo §9: `peripheral_fatigue` → "pernas pesadas", "braços falhando";
  `central_fatigue` + sono → "difícil manter foco", "reação atrasada"; `hydration` → sede, boca seca,
  tontura; `energy` → fome, fraqueza, tremor; `injury.severity` → dor localizada, instabilidade,
  **frequentemente subestimando**.
- **`SelfPhysicalModel` como crença**, vivendo em Agent Knowledge & Memory e **não** no world state —
  a separação de árvore do snapshot é estrutural, não estilística.
- **Viés genuinamente grande, inclusive no melhor caso.** Acurácia interoceptiva é individualmente
  muito variável e mal medida; até os testes-padrão de percepção cardíaca são contestados, com parte
  dos indivíduos respondendo por fase cardíaca em vez de por detecção. Modelar autopercepção como
  quase verídica é dar telemetria por outro nome. O sinal é do tipo RPE, e colhido durante ou logo
  após o esforço tende a **superestimar** a fadiga.
- **`body_awareness` e analgesia.** Alta consciência corporal reduz o viés sem zerá-lo; adrenalina e
  medicação suprimem o sinal **sem curar tecido**, e o engine, que conhece a verdade, aplica o
  agravamento mesmo assim.
- **`perceived_effort`** ordinal, enviesado — e **nunca** igual ao `target_intensity` que o engine
  usou.
- **Snapshot: a árvore `physical_beliefs`.** `self_physical_model` e `capacity_beliefs` por holder,
  em árvore separada de `characters`. Um serializador capaz de escrever crença dentro de
  `characters.<id>` já perdeu a invariante 1, e o teste é sobre a capacidade, não sobre o resultado.
- **`interoception-params.yaml`.** Viés por `body_awareness`, ganho de analgesia, mapa canal →
  vocabulário qualitativo.
- **Esqueleto versionado da família de evals `knowledge-boundary`** (spec §11.6). Fica **vazia de
  comportamento dependente de modelo**, porque a V1 não tem nenhum. Ela existe para bloquear a
  integração do Agent Cognition, não esta: quando o Agent Cognition entrar, o eval verifica que nenhum
  prompt contém número de `BodyState` e que um agente não consegue reproduzir sua própria capacidade
  nominal quando perguntado diretamente.

## Arquivos e módulos

```
src/embodiment/{types,interoception,snapshot,cli}.py
data/models/physical/interoception-params.yaml   novo
evals/knowledge-boundary/                        novo — esqueleto versionado, sem casos de LLM
tests/embodiment/{test_interoception,test_self_physical_model,test_belief_tree_separation}.py
tests/embodiment/properties/test_p7_no_telemetry.py
tests/embodiment/invariants/test_invariant_{06,12}_*.py
tests/embodiment/scenarios/test_scenario_04_concealed_injury.py
tests/embodiment/boundary/test_leak_classes.py
```

## Decisões que consome

Nenhuma da §13.

## Testes determinísticos

- **Mapa canal → vocabulário completo.** Todo canal do modelo §9 produz sinal; um canal sem tradução
  é vazamento por omissão, porque o agente deixaria de sentir algo que o corpo tem.
- **Viés grande no melhor caso.** Mesmo com `body_awareness` máximo, o erro entre sinal e verdade fica
  acima de um piso declarado em `interoception-params.yaml`. Um teste falha se algum ajuste de
  parâmetro tornar a autopercepção quase verídica.
- **Direção do viés.** Lesão tende a ser subestimada; fadiga colhida durante ou logo após o esforço
  tende a ser superestimada. As duas direções são asseridas separadamente, porque são fenômenos
  diferentes.
- **Analgesia suprime sinal sem curar tecido.** Com analgesia alta, o sinal cai e o `BodyState` não
  muda; e o agravamento por carga continua sendo aplicado pelo engine.
- **`perceived_effort ≠ target_intensity`**, para todo estado.
- **Separação de árvore.** Um teste tenta escrever `self_physical_model` dentro de
  `characters.<id>` e o serializador **recusa**. Testar que "por acaso não escrevemos lá" não vale.
- **Determinismo da tradução.** Mesmo `BodyState` e mesmo substream produzem o mesmo sinal.

## Property tests

- **P7 — sem telemetria.** Para **qualquer** `BodyState`, o payload de contexto gerado por
  `interoception.py` não contém nenhum número presente no estado. Gerado com `hypothesis` sobre
  estados válidos arbitrários, comparando por valor e não por chave — um número reformatado continua
  sendo o número.

## Cenários

**4 — lesão ocultada.** Torce o punho e esconde. Verdade: lesão, impedimento por dimensão e risco de
agravamento. Autopercepção: subestima. Terceiros: nada, até uma pista vazar — e emitir pista depende
de dor e de esforço de ocultação, que **custa fadiga central**. Semanas depois, uma tarefa que exige
preensão decide o exame, e nenhuma cena precisou narrar isso. O teste percorre as três camadas em um
run só, com seed fixo.

Não é licença dramática: entre atletas universitários com histórico de concussão, 43% relataram ter
escondido sintomas deliberadamente para continuar jogando. Ocultar lesão é a linha de base, não a
exceção.

## Checagens de fronteira de conhecimento

Este ticket **é** a checagem. As cinco classes da spec §7.3, cada uma com teste nomeado:

1. **Telemetria corporal própria em prompt** — coberta por P7.
2. **`BodyState` de terceiro em prompt** — nenhum caminho de contexto lê o estado de outro; o único
   canal é `ObservedPerformance` (PSV1-5).
3. **Resultado do sorteio de detecção visível ao ocultador** — verificada no PSV1-5, reasserida aqui
   sobre o payload de contexto, que é onde ela apareceria.
4. **Posterior de capacidade recuperado por RAG** — o corpus de feats é paráfrase; o posterior não é
   documento. Teste: nada em `capacity_posterior` ou `capacity_baseline` está em caminho indexável.
5. **Feat posterior à divergência aparecendo como memória** — o mesmo registro é **legítimo no
   seeding e proibido na memória**. É a única estrutura do repositório com essa assimetria, e portanto
   a que mais provavelmente será implementada errada; o teste usa o mesmo feat nos dois caminhos e
   assere aceitação num e recusa no outro (invariante 12, cenário 6).

A skill `knowledge-boundary-audit` roda sobre o pacote inteiro e o resultado entra na evidência de
conclusão. Um achado é bloqueante, não uma observação.

## Fora do escopo

Qualquer eval que exija um LLM — a família fica versionada e vazia. Context builder de agente, que
pertence ao Agent Cognition; este ticket entrega o `SelfPhysicalModel` que ele vai consumir e para no
limite do subdomínio.

## Evidência de conclusão

1. O comando da seção de resultado imprime as duas colunas, e a coluna do agente não contém dígito
   algum vindo do `BodyState`.
2. P7 verde sob `hypothesis`, comparando por valor.
3. Os cinco testes de classe de vazamento verdes, nomeados pelas classes da spec §7.3.
4. `knowledge-boundary-audit` executada, com relatório anexado ao ticket, sem achados.
5. O cenário 4 passa com seed fixo, percorrendo verdade, autopercepção e terceiros.
6. `evals/knowledge-boundary/` existe, versionada, com README explicando por que está vazia e o que
   a preencherá.
