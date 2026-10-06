# COTE Causal Canonical Codec V1 — policy bundle

**Status:** Normativo

**Policy:** `cote.csf.codec.cbor-det.v1`

**Decisão:** [ADR 0009](../../adr/0009-canonical-causal-codec-and-digests.md)

Este diretório é a autoridade imutável e independente de linguagem para o codec V1. Prose no ADR,
no contrato arquitetural ou na spec explica a decisão; em caso de divergência sobre bytes, prevalece
este bundle na seguinte ordem:

1. `profile.json`, para regras globais do encoder, texto, digest e limites;
2. `foundation.cddl`, para forma e ordem de campos;
3. `registries.json`, para domain tags, schemas, enums, variants, roles e coleções set-like;
4. `causal-transition-contracts.json`, para regras executáveis entre records e estados duráveis;
5. `unicode.json`, para a versão e os dados Unicode exatos;
6. `fixtures.json` e `causal-transition-fixtures.json`, para bytes/digests e transições positivas e
   negativas;
7. `policy-manifest.json`, `schema-manifest.json` e `conformance-manifest.json`, para a composição e
   os hashes do bundle.

Todos os arquivos são UTF-8 sem BOM, com LF e newline final. Paths são ASCII, relativos a este
diretório, sem `.`/`..`, barra inicial ou barra invertida. O byte content dos artefatos é normativo;
JSON e CDDL aqui não são usados como preimage causal de objetos de uma run.

## Bootstrap dos hashes

Para evitar ciclo entre o codec e o hash da própria policy, o bootstrap usa SHA-256 diretamente
sobre bytes de arquivo:

```text
artifact_hash(path) = SHA-256(raw_file_bytes(path))

codec_policy_hash = SHA-256(
  ASCII("cote.csf.bundle.codec-policy.v1") || 0x00 || raw_file_bytes("policy-manifest.json")
)

schema_bundle_hash = SHA-256(
  ASCII("cote.csf.bundle.schema.v1") || 0x00 || raw_file_bytes("schema-manifest.json")
)

conformance_suite_hash = SHA-256(
  ASCII("cote.csf.bundle.conformance.v1") || 0x00 || raw_file_bytes("conformance-manifest.json")
)
```

Os manifestos são arrays JSON ordenados por `path`; cada entrada fixa `path`, `size` e
`sha256_lower_hex`. O verificador precisa rejeitar path repetido, item ausente/extra, tamanho ou hash
divergente e bytes JSON diferentes dos commitados. Os três hashes resultantes estão em
`BUNDLE.sha256`; esse arquivo é um recibo e não entra em nenhum deles.

`policy-manifest.json` contém somente `profile.json` e `unicode.json`: regras globais cuja mudança
cria codec V2. `schema-manifest.json` contém `foundation.cddl`, `registries.json` e
`causal-transition-contracts.json`: regras locais versionáveis sem mudar o encoder.
`conformance-manifest.json` contém os dois arquivos de fixtures. Separar a suíte
do schema evita autorreferência: o vetor de genesis pode persistir o `schema_bundle_hash` real sem que
seus próprios bytes integrem esse hash. Assim um novo schema/domain tag altera apenas um
schema/extension bundle e exige novo genesis quando não estava pinado, mas não cria falsamente um
novo codec.

Depois do bootstrap, ids e digests causais usam exclusivamente o envelope CBOR e `digest_v1` do
contrato. O bootstrap não autoriza hash genérico para artefatos causais.

## Escopo dos schemas

`foundation.cddl` fecha os records da fundação exigidos pelo ADR 0008. Um `typed-value` de domínio é
um envelope canônico completo, armazenado como bytes, acompanhado do digest desse envelope; assim o
record externo não depende da representação de objeto da linguagem. Action types, event types,
payloads de occurrence, predicates e substates de domínio são extensões e precisam estar em um
extension bundle imutável listado no genesis antes de aparecerem numa run. Extension bundle ausente,
schema desconhecido ou bytes que não reencodam identicamente falham fechado.

Em `typed-value`, `schema-id` e `schema-version` externos precisam ser iguais aos campos 4 e 5 do
envelope interno. O `domain-tag` interno precisa estar registrado pela extensão para aquele papel, e
`envelope-digest` é SHA-256 dos bytes exatos do envelope. Divergência de tag/schema/version/digest,
tag reutilizada para outro papel ou envelope não strict-canonical falha fechado antes de o record
externo ser aceito. `typed_value_constraints` representa essas relações em dados cobertos pelo
`schema_bundle_hash`; os seis casos `typed_value_constraint` exercitam a aceitação e cada falha
isoladamente.

Uma extensão pode adicionar schema/domain tag, enum ou role sem alterar este diretório, mas não pode
redefinir nome/código existente nem relaxar o profile. O genesis fixa o hash de cada extension bundle
e o conjunto de pares `(schema_id, schema_version)` admitidos sob o `schema_bundle_hash`; não existe
hash local de schema sem algoritmo próprio. Uma run não incorpora bundle publicado depois do genesis.
`causal-ref` e `evidence-ref` usam os códigos de `reference_kind`, com exatamente um root persistido
por código e um código por root. `input-ref` usa `unit_kind`; sua branch `slot` inclui também
`slot_response_kind`, portanto todos os dispatches são reconstruíveis dos próprios bytes.
Roles/códigos extension-specific só são válidos quando o extension bundle requerido pelo registry os
declara de forma append-only.

`enum_bindings` e `role_bindings` ligam cada field path ao registry aplicável; binding do path mais
específico prevalece sobre o binding do tipo reutilizado, e `u8`/`u16` sozinho nunca autoriza um
código. Isso inclui os fields próprios dos preimages registrados: nenhum preimage herda
informalmente o binding do record que o originou. `reference_identities` liga todos os 28
`reference_kind` ao seu único root persistido e às operações de id e digest.
`unit_reference_dispatch` e `slot_response_reference_dispatch` fecham os dois dispatches
discriminados de `input-ref`. Cada entrada de
`set_like_collections` tem quatro posições normativas — field path, ordering key, duplicate policy e
identity key —; mesma identity key com bytes diferentes é sempre `SET_IDENTITY_COLLISION`.

`derived_ordering_components` fecha qualquer componente de ordenação calculado. Em particular,
`indeterminate-evidence-refs-digest` normaliza `evidence_refs` como
`cote.csf.schema.indeterminate-evidence-ref-list` e aplica a operação registrada
`cote.csf.digest.indeterminate-evidence-refs`; hash de array nu ou envelope inventado é inválido.

`Event` e `SourceClosure` persistem `run_id`, portanto seus ids são recomputáveis sem contexto do
store. Cada `admitted-unit` persiste `eligibility`, `unit-kind` e `source-id` além de id/digest; sua
ordering key inteira é obtida do próprio valor canônico e conferida contra o record do ledger.

`fixture_domain_operations` registra separadamente todas as combinações
`(domain_tag, schema_id, schema_version)` usadas pelos casos positivos. Strict decode em modo de
conformidade aceita essa tabela além de `domain_operations`; uma run de produção nunca a carrega nem
aceita o prefixo `cote.csf.test.*`.
`fixture_roles` cumpre a mesma função para códigos de role extension-specific usados pelos roots de
teste; fora do modo de conformidade, ausência do registry da extensão continua `UNKNOWN_ROLE`.

`RoundDeclaration`, `ScheduledOccurrence`, `Claim` e `Transmission` persistem origin, role e local
ordinal requeridos por seus preimages de id; o primeiro restringe origin a evento ou genesis.
`IdempotencyIdentity` é um root persistível separado e só existe quando o optional
`idempotency-key` da unidade está PRESENT. Seu preimage fecha run, kind, producer/actor scopes, key,
operação lógica e policy ref.

## Vetores e verificação independente

**Execução de conformidade neste checkpoint:** pendente. Este checkpoint arquitetural publica o
contrato e os vetores, mas não inclui dois runners independentes nem um workflow que os execute. Uma
implementação da fundação não pode alegar conformidade até satisfazer esse gate em seu CI.

`fixtures.json` contém 156 vetores positivos, 30 negativos, 306 casos semânticos, sete casos de
normalização convergente e dois vetores SHA-256. `causal-transition-fixtures.json` acrescenta 101 casos
de transição executáveis, totalizando 602 vetores/casos de conformidade. Os positivos cobrem primitivos e limites, Unicode,
map/list/set, floats, todos os roots persistidos e as 76 operações de id/digest/hash/record do registry.
Cada um fixa payload CBOR, envelope completo e SHA-256. Os negativos fixam bytes/input e
`error_code` estável. Os casos semânticos executam bindings de enum em roots completos, subsets de
`reference_kind` por substituição tipada sobre um root-template completo, duplicata/colisão de cada
família de ref, record persistido → ref para todos os 28 roots referenciáveis (inclusive dispatches
de input e slot response), dois attempts do mesmo ciclo, permutação de `rule-versions`, fence completo
permutado,
mismatch/corrupção entre fence e ledger, bindings dentro de envelopes de operação, o digest
domain-separated de evidência indeterminada, pinning do genesis e idempotência
ABSENT/PRESENT/conflitante.

`record_constraints` fecha as cinco variantes de `DecisionRecord`, os quatro status de
`CycleControlState`, a cadência condicional de `TriggerDefinition` e os ordinais/retry refs de
`AdmissionFence`/`AttemptRetryRecord`. `persisted_roots` é a lista fechada dos 34 records persistidos,
inclusive revogação de dispatch, definition/runtime de trigger, controle de ciclo e identidade de
idempotência. `record_authorities` atribui owner a cada um, incluindo input ledger, schedule store,
trigger registry, decision ledger, event/evidence stores, causal outbox, world state e stores de
snapshot/genesis. Em `linked_record_constraints`, cada entrada declara
uma transição de append ou publicação atômica, o root candidato, o cursor e `record_roles` com fonte e
cardinalidade. `causal-transition-contracts.json` torna normativas as fronteiras de append de fonte,
dispatch/resposta, fence, terminal commit, genesis e snapshot e a dobra de controle. Seus casos usam
JSON Pointer e apenas `add/remove/replace`; o runner materializa integralmente cada estado antes de
executar a regra, portanto labels de mutação não contam como evidência. Os 48
`linked_record_scenarios` incluem oito estados positivos e 40 inputs duráveis
completos rejeitados; os demais negativos fornecem o record CBOR substituto exato.
A sequência fence abortado → abort → retry → novo fence é validada em três fronteiras duráveis,
preserva byte a byte plano, base, closure, coorte, unidades, input e policies fora dos quatro campos
que a retentativa pode mudar, e vincula `rule-versions` do retry ao novo fence.
`CommitCandidate`, `ProvisionalDisposition`, `ConflictSet` e `RngDraw` são material provisório
imutável já durável no `DecisionLedger`; o terminal apenas os resolve. Inputs imutáveis são
liquidados por `DecisionRecord` + `CycleCommit`, enquanto agenda e trigger exigem transição do reducer
de um evento `SOURCE_LIFECYCLE`. Essas provas, sucessores de `DEFER`, decisões, eventos,
runtime/ativações de trigger, tasks e redução pinada de mundo são candidatos da mesma publicação
indivisível, com coordenadas comuns e bijeção/digests de
settlement. Witnesses CBOR tipados provam pre/post-state das fontes e do mundo, incluindo a dobra
dos reducers por owner/version/hash, cursor lógico contíguo derivado do último commit ou genesis —
inclusive lote vazio — e classificação perceptiva bijetiva sob a policy pinada;
`perception_policy_contracts` fixa o predicado versionado usado pelos cenários e seu hash;
observation → knowledge
input → completion é validado depois nas suas fronteiras de append, repete identity policy/resolver
pinados e restringe cada
observation ao evento da task, projeta apenas claims/evidence divulgados e exige no completion
exatamente todos os outputs já presentes no prefixo daquela task. Completion fecha a task e bloqueia
qualquer observation ou knowledge input tardio. `RngDraw` é um root referenciável
próprio, e os subsets de field path impedem que uma ref genérica ocupe
`rng-draw-refs` ou `failure-evidence-refs`. A evidência de abort admite somente ConflictSet, sorteio,
assessment, disposição provisória e falha de tentativa, todos com identidade e digest totais.
O resultado de `RngDraw` é ainda derivado da policy pinada sobre `world_seed` e a tuple de substream;
digest local não substitui essa prova. O snapshot carrega seed/hash, genesis, policies de replay e os
cinco cursores fixos. Checkpoints epistemológicos usam SHA-256 dos bytes opacos exatos, resolvidos
antes do hash quando há locator. `parent_checkpoint_history_ref` PRESENT é parseado e verificado no
append do genesis pelo framing Cross-policy V1.
`AttemptFailure` inclui stage e component policy no preimage; seu ordinal é nomeado pelo schema do
componente e não pode vir de completion order.

No append de abort, sujeitos e failure evidence resolvem no mesmo fence/attempt: admitted units
repetem id/digest do corte, candidatos pertencem à sua partição, artefatos coordenados repetem
run/cycle/attempt, assessments resolvem sujeito do corte e conflict sets somente candidatos dele.

Checagens temporárias usadas durante a autoria não foram preservadas e não contam como evidência de
conformidade reproduzível. A aceitação de uma implementação exige dois runners preservados, em
linguagens diferentes, executando positivos, negativos, normalização e semântica conforme o contrato
principal.

## Classificação de texto

Todo campo textual do schema é exatamente uma destas classes normativas:

- `machine-id`: identidade, namespace, role, reason code, policy id ou chave causal;
  ASCII minúsculo, 1–128 octetos, sob a ABNF integral de `profile.json` — inicia por `[a-z]` e cada
  separador em `._:/-` é seguido por um ou mais `[a-z0-9]`;
- `domain-tag`/`schema-id`: identificadores ASCII sob `domain-schema-tag`, a ABNF mais restrita que
  exige pelo menos dois segmentos separados por ponto e permite hífen somente dentro de segmento;
- `iana-timezone`: nome IANA sob a ABNF específica do profile, com case significativo, como
  `Asia/Tokyo`;
- `human-text`: conteúdo humano; Unicode scalar values, NFC pelo Normalization Process for
  Stabilized Strings (NPSS) de Unicode 15.1.0, sem code point unassigned nessa versão;
- `ascii-uri`: locator não autoritativo, ASCII visível sob a gramática registrada no schema.

O CDDL usa aliases `tstr` para essas classes e delega a validação lexical ao ABNF de `profile.json`.
Isso é intencional: `.regexp` do RFC 8610 usa a linguagem de regex XML Schema, portanto padrões em
sintaxe PCRE/ECMAScript seriam não portáveis. Validar somente o tipo CDDL, sem aplicar o profile e os
dados Unicode, é não conforme.

Da mesma forma, `causal-ref` é a forma estrutural comum, mas não abre todos os roots em todo campo.
`enum_bindings` seleciona subsets exatos de `reference_kind` para refs semanticamente tipadas; o
decoder aplica tanto o binding estrutural comum quanto o binding mais específico do field path antes
de aceitar a ref.

`machine-id` e `domain-tag`/`schema-id` não aceitam maiúscula, espaço, controle, bidi,
default-ignorable ou qualquer non-ASCII; logo não dependem de detecção de script/confusable.
`iana-timezone` aceita maiúsculas somente conforme sua ABNF própria, não como `machine-id`.
`human-text` nunca pode ser domain tag, schema id, map key, identity key ou ordering key. NFC/NPSS
estabiliza bytes de conteúdo; não é apresentado como controle de spoofing de identificadores.

## Extensão e mudança

Os registries são append-only. Alterar uma entrada existente é proibido. Nova versão de schema é
necessária para campo, layout de record, range, optional, enum/variant/role local, limite local ou
ordering/duplicate policy de uma coleção. O bundle imutável da run precisa já conter essa versão; caso
contrário, seu uso exige novo genesis/fork.

Nova policy/`canonical_bytes_v2` é necessária para envelope, subset ou deterministic rules CBOR,
representação de primitivo, versão/repertoire Unicode, gramática global de identifiers/domain tags,
ordem global de map, regra global de float, algoritmo/formato de digest ou forma textual de digest.
Adicionar um schema/domain tag novo sem mudar essas regras cria novo bundle para um novo run, mas não
transforma o encoder V1 em V2.
