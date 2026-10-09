# CSFRV1-1 — Genesis e contrato causal canônico executável

**Spec:** §6.1–6.3 · §7.1 · §11.1 (1–3, 15–17) · §12.3  
**ADR 0008:** invariantes 14, 33 e partes canônicas de 34/39  
**Bloqueado por:** CSFRV1-0  
**Bloqueia:** CSFRV1-2

## Resultado observável

O harness cria um run vazio a partir de um manifesto, valida todos os pins de policy/schema/Unicode,
serializa records do bundle, deriva ids/digests domain-separated e recusa qualquer byte não
canônico. Dois runners independentes, em linguagens diferentes, executam os 620 casos normativos e
produzem o mesmo relatório de conformidade.

## Escopo

- Esqueleto do runtime e harness, sem framework de agentes ou cliente LLM.
- `Genesis`, policy registry, `SimulationInstant`, `EligibilityCoordinate`, causal refs e policy refs.
- Encoder/strict decoder CBOR determinístico V1; NPSS Unicode 15.1; SHA-256 domain-separated.
- `derive_id`, digests e state/envelope hashes apenas pelas operações registradas.
- Verificação dos três manifests/hashes do bundle e carregamento fail-closed de versões.
- Interfaces e stores append-only mínimos em memória para exercitar records, sem fingir a topologia
  persistente do CSFRV1-5.
- Segundo runner independente preservado no repositório e workflow que compara os relatórios.

## Arquivos e módulos prováveis

Os paths exatos são atualizados pelo CSFRV1-0; responsabilidades esperadas:

```text
runtime/csf/{codec,identity,genesis,policies,types,stores,harness}
tools/csf-conformance-runner-2/
tests/csf/{codec,identity,genesis,architecture}/
.github/workflows/csf-conformance.*
```

O bundle normativo é consumido sem alteração.

## Testes determinísticos e de propriedade

- 620 casos positivos, negativos, semânticos e de transição passam nos dois runners.
- Primitivos, inteiros-limite, floats, maps, sets, records, refs e texto produzem golden bytes.
- Strict decode rejeita CBOR não preferido, indefinite, tags, `undefined`, duplicate key, `-0.0`,
  não-finito, Unicode fora do profile, schema/enum desconhecido e digest/id de tamanho errado.
- Mesmo valor sob tag/schema/version diferentes produz bytes/digests diferentes.
- Permutar map/set não altera bytes; identity collision com bytes divergentes falha fechado.
- Genesis sem qualquer pin/hash obrigatório, timezone distinto do declarado ou
  `next_logical_sequence != 0` é recusado.
- Teste de arquitetura proíbe cliente LLM, RNG global, UUID aleatório e wall clock na identidade.

## Evals e fronteira de conhecimento

Nenhum eval de LLM/RAG. O registry de autoridades é testado estruturalmente: world, decision,
evidence, outbox e snapshot têm owners distintos e o harness não oferece query global a agentes.

## Fora do escopo

Ingresso, fence, decisão, reducers, persistência de produção, percepção e replay. Não criar schemas
locais alternativos aos records já fechados pelo bundle.

## Evidência de conclusão

1. Relatórios dos dois runners identificam os mesmos 620 casos e ficam verdes no CI.
2. Um genesis válido é persistido e reencoda byte a byte; corrupção de um pin impede abertura.
3. Golden ids/digests batem com `BUNDLE.sha256` e fixtures.
4. Testes de arquitetura provam ausência de LLM, UUID/wall-clock identity e hash genérico causal.
