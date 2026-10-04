# Canonical Causal Codec V1

**Status:** Normativo

**Decisão:** [ADR 0009](../adr/0009-canonical-causal-codec-and-digests.md)

**Base:** [RFC 8949](https://www.rfc-editor.org/rfc/rfc8949.html) core deterministic encoding,
[RFC 8610](https://www.rfc-editor.org/rfc/rfc8610.html) CDDL,
[Unicode 15.1.0](https://www.unicode.org/versions/components-15.1.0.html) com
[UAX #15 revision 54](https://www.unicode.org/reports/tr15/tr15-54.html) e
[FIPS 180-4 SHA-256](https://csrc.nist.gov/pubs/fips/180-4/upd1/final)

**Bundle normativo:** [COTE Causal Canonical Codec V1 bundle](canonical-codec-v1-bundle/README.md)

Este documento fixa os bytes de `canonical_bytes_v1` exigidos pelo ADR 0008. “CBOR canônico”, sem
este profile, não é uma implementação conforme.

## 1. Identidade da política

| Campo | Valor V1 |
|---|---|
| `codec_policy_id` | `cote.csf.codec.cbor-det.v1` |
| `codec_version` | `1` |
| formato | CBOR, RFC 8949 core deterministic encoding (§4.2.1) |
| schema notation | CDDL, RFC 8610, mais as restrições semânticas deste profile |
| Unicode | NFC pelo Normalization Process for Stabilized Strings (NPSS), Unicode `15.1.0` |
| hash | SHA-256, FIPS 180-4, saída de 32 bytes |
| `identity_algorithm_version` | `cote.csf.sha256.v1` |
| `codec_policy_hash` | `513111dc82a5e58c07aecdabf410633f5aa5418908d2461ef0dff0d9ae5d8203` |
| `schema_bundle_hash` | `185928983d156e477e242c665e1fbd84cfcf632e542e9b6ef830731e60b58cd1` |
| `conformance_suite_hash` | `bf07c9d3ff824a299bf5a656bc24fc9d1d66da9b87ea5dbc3e61c613ad5579b5` |

O manifesto imutável do run carrega os campos de policy/profile e o `schema_bundle_hash`; este último
cobre os registries de domain tags, enums, ordering keys e duplicate policies. O
`conformance_suite_hash` é recibo externo da suíte que valida o bundle, não campo causal do genesis —
incluí-lo no próprio vetor criaria autorreferência. Um nome igual com hash diferente é corrupção.

## 2. Envelope canônico

A operação normativa é:

```text
canonical_bytes_v1(domain_tag, schema_id, schema_version, value) =
  deterministic_cbor([
    h'43534600',       // byte string ASCII "CSF" + NUL; não text string
    1,                 // codec_version
    domain_tag,        // text ASCII registrado
    schema_id,         // text ASCII registrado
    schema_version,    // uint
    normalize(value)   // payload conforme schema
  ])
```

O envelope é um array CBOR de comprimento exatamente seis. Nenhum campo pode ser omitido, inferido do
nome do arquivo ou substituído por header de transporte.

`domain_tag` e `schema_id` usam somente ASCII minúsculo e seguem a ABNF
`domain-schema-tag` de `profile.json`: dois ou mais segmentos separados por `.`, cada segmento
iniciado por `[a-z]`, continuado por `[a-z0-9]` e com hífens internos sempre seguidos por pelo menos
um `[a-z0-9]`. Produção reserva o prefixo `cote.csf.`; fixtures de conformidade usam
`cote.csf.test.`. Tags e ids de schema são registries append-only: um nome nunca é reutilizado com
outro significado.

Cada operação de id ou digest possui domain tag próprio. Exemplos:

```text
cote.csf.id.event
cote.csf.id.commit-candidate
cote.csf.digest.action-proposal-unit
cote.csf.digest.fence
cote.csf.digest.decision
cote.csf.digest.batch
cote.csf.digest.abort
cote.csf.hash.world-state
cote.csf.hash.snapshot
```

Não existe domain tag genérico `cote.csf.hash`. Acrescentar um tipo causal exige tag nova no registry
e novo hash da policy bundle. `schema_id + schema_version` também entra no preimage: valores de tipos
diferentes não compartilham namespace apenas porque possuem os mesmos campos.

## 3. Primitivos

| Tipo do schema | Representação canônica |
|---|---|
| `null` | CBOR simple value `null` (`0xf6`), somente onde o schema declara null semântico |
| `bool` | `false` (`0xf4`) ou `true` (`0xf5`); integers `0/1` não são coerção válida |
| unsigned integer | major type 0, preferred/shortest encoding; range `u8/u16/u32/u64` validada pelo schema |
| signed integer | major type 0 ou 1, preferred/shortest encoding; range `i8/i16/i32/i64` validada pelo schema |
| float | valor semântico IEEE-754 binary64 finito, CBOR float no menor width que preserve exatamente o valor |
| byte string | major type 2, definite length |
| `human-text` | NFC/NPSS, UTF-8 válido, major type 3, definite length |
| `machine-id` | ASCII minúsculo sob a gramática fechada do bundle, major type 3, definite length |
| `domain-tag`/`schema-id` | ASCII minúsculo sob a ABNF `domain-schema-tag`, mais restrita que `machine-id` |
| `iana-timezone` | nome IANA sob a ABNF versionada do bundle; maiúsculas de nomes como `Asia/Tokyo` são significativas |
| enum | código unsigned estável declarado no registry do schema |
| timestamp | `SimulationInstant`: `i64` de microssegundos desde Unix epoch UTC; nenhum tag ou texto de data |
| duration | integer de microssegundos com sinal/range declarado pelo schema |
| derived id/digest | byte string de comprimento exatamente 32 em campo tipado |

### 3.1 Integers e valores exatos

O width e sinal pertencem ao schema, embora o CBOR use a forma mais curta do valor matemático. Assim,
`u8(1)` e `u64(1)` têm o mesmo item CBOR, mas aparecem sob schemas/campos diferentes e não são
intercambiáveis. Valores fora do range falham antes do encoding. Tags CBOR 2/3 de bignum são proibidos
na V1.

Decimal exato, dinheiro, score e unidade física que exija igualdade cross-language usam integer
coefficient com scale e unidade fixados no schema. CBOR decimal fraction (tag 4), string decimal e
decimal nativo de linguagem são proibidos na imagem canônica V1.

### 3.2 Floats

O schema só oferece o tipo semântico `f64`; não há input `f16`/`f32`. O encoder aplica:

1. rejeitar `NaN`, `+Infinity` e `-Infinity`;
2. normalizar `-0.0` para `+0.0`;
3. preservar integer e float como tipos distintos — `1` e `1.0` nunca são coercidos;
4. usar o menor encoding binary16/binary32/binary64 que round-trip exatamente ao valor binary64,
   conforme RFC 8949 preferred serialization.

O decoder estrito rejeita non-finite, negative zero e float em encoding mais largo que o necessário.
Subnormals finitos são válidos. Esta regra estabiliza bytes; ela não promete que algoritmos numéricos
diferentes produzam o mesmo resultado. Regras de mundo que exigem igualdade aritmética usam integer ou
fixed-point; integrações que usam float precisam de determinism eval próprio.

### 3.3 Texto, Unicode e identificadores

Antes do UTF-8:

1. validar uma sequência de Unicode scalar values — surrogate isolado é inválido;
2. aplicar NFC pelo Normalization Process for Stabilized Strings de Unicode 15.1.0;
3. rejeitar qualquer code point `General_Category=Unassigned` nessa versão;
4. emitir UTF-8 shortest form, sem BOM e sem escape textual.

Para `human-text`, comparação e tamanho usam os bytes UTF-8 **depois** dessa normalização. Case folding,
compatibility normalization (NFKC), locale e collation não são aplicados.

Atualizar a biblioteca Unicode do host não muda o run: a implementação usa os dados 15.1.0 ou prova
conformidade com eles. O bundle prende UAX #15 revision 54, `NormalizationTest.txt` e
`DerivedGeneralCategory.txt` por URL, tamanho e SHA-256; este último define exatamente
`General_Category=Unassigned (Cn)`.

`human-text` é conteúdo, nunca identidade causal, map key ou ordering key. Identificadores textuais
causais comuns usam `machine-id`: 1–128 octetos ASCII sob a ABNF integral de `profile.json`; começa por
`[a-z]` e cada separador em `._:/-` precisa ser seguido por um ou mais `[a-z0-9]`. As subclasses
`domain-tag`/`schema-id` usam a gramática mais restrita da §2. `iana-timezone` é uma classe textual
de configuração separada, com sua própria ABNF e case significativo. Isso exclui controles bidi,
default-ignorables, mistura de scripts e homoglyphs non-ASCII dos identificadores por construção.
NFC/NPSS estabiliza conteúdo Unicode; não é uma política de segurança de identifiers. Todo campo
textual precisa ser classificado no schema como `machine-id`, `domain-tag`/`schema-id`,
`iana-timezone`, `human-text` ou `ascii-uri`; `text` sem classe é inválido.

### 3.4 IDs e digests

Dentro de records tipados, `Id<T>` e `Digest<T>` são byte strings de 32 bytes. Hex, base64, UUID ou
string prefixada nunca entra no preimage no lugar desses bytes.

Para CLI, logs e documentos, a única forma textual é:

```text
csf1:<domain-tag>:sha-256:<64 lowercase hex digits>
```

Essa forma é view. Um adapter a converte para domain tag validado + 32 bytes antes de chamar o codec;
o texto em si não é hashado. Uppercase, prefixo/algoritmo diferente, hex de tamanho errado ou domain
tag incompatível com o campo são rejeitados.

`causal-ref = [reference_kind, id, digest]` não admite identidade implícita. Cada código de
`reference_kind` seleciona em `registries.json.reference_identities` exatamente um root persistido e
as operações, schemas e componentes tanto do id quanto do digest; roots distintos nunca possuem
aliases de `reference_kind`. `input-ref`, que é o índice genérico das unidades admitidas, usa
`[unit_kind, id, digest]` para branches não-slot e
`[slot, slot_response_kind, id, digest]` para distinguir `ActionProposal` de `NoProposal` sem consulta
ao store. As tabelas normativas `unit_reference_dispatch` e `slot_response_reference_dispatch`
resolvem somente esse `input-ref`. Strict replay recompõe toda ref a partir dos próprios bytes e do
record persistido; metadado de chamada/store não pode completar um discriminante ou preimage ausente.

## 4. Records, opcionais e sum types

Record é array CBOR de tamanho fixo, com campos na ordem declarada pelo CDDL/schema. Nomes de campo
nunca entram nos bytes do record. Array curto, campo extra e versão desconhecida são erros.

Ausência não é `null`. Campo opcional usa sum type explícito:

```text
[0]          // ABSENT
[1, value]   // PRESENT
```

Se o domínio precisa distinguir `PRESENT(null)`, ele aparece como `[1, null]`. Outros discriminated
unions começam por enum uint registrado e têm aridade fixa por variante. Ordinal local e role
declarados por schema seguem a mesma regra: códigos são estáveis e nunca vêm do ordinal de enum da
linguagem.

`registries.json` liga cada field path ao registry de enum/variant ou ao registry de role exigido da
extensão. Um `u8`/`u16` nu no CDDL não autoriza qualquer código: strict decode resolve o binding pelo
path e rejeita código ou role ausente. Campos `idempotency_key` usam o optional discriminado; somente
`PRESENT(machine-id)` produz `IdempotencyIdentity`, enquanto `ABSENT` não produz alias nem sentinela.
O mesmo vale para `causal-ref`: campos semanticamente tipados resolvem um subset exato de
`reference_kind` pelo path. Assim closure, resposta de slot, retry/abort, fence/envelope terminal,
conflict set, observation e claim refs não aceitam outro root apenas porque ele é uma ref canônica
válida em contexto genérico. O binding do field path mais específico refina o binding estrutural de
`causal-ref.kind`; strict decode exige ambos, portanto o subset nunca é alargado pelo alias comum.

`record_constraints` fecha invariantes locais condicionais que CDDL de aridade fixa não expressa
sozinho; `typed_value_constraints` fecha a validação do envelope interno sob o mesmo schema bundle
hasheado; `record_authorities` fixa o owner de cada root entre decision ledger, event store, evidence
ledger e causal outbox; `linked_record_constraints` fecha invariantes referenciais por transição de
append contra o prefixo durável anterior ou pela publicação atômica do commit batch.
Para `DecisionRecord`, o decoder seleciona a variante por `disposition` e valida presença, ausência e
vazio de candidate, conflicts, successor e canonical unit antes de aceitar o root. Para
`CycleControlState`, fecha presence, kind do último envelope e relações de ordinal dos quatro status;
para `TriggerDefinition`, exige cadência PRESENT e positiva somente em `REPEAT_WHILE_TRUE`. O primeiro
fence exige attempt 1 sem retry; fences posteriores exigem `retry-ref`; cada `AttemptRetryRecord`
avança exatamente um ordinal e resolve o último abort do mesmo run/cycle no cursor pre-append. O
abort resolve o fence anterior e repete seus plano/digests; o novo fence resolve o retry, exige
`rule_versions` igual ao do retry após normalização canônica e é byte a byte igual ao fence
abortado, exceto por `attempt_ordinal`, `retry_ref`, `rule_versions` e o `fence_digest` resultante.

A relação `Event` → `PerceptionTask` nasce somente dentro da publicação indivisível que também inclui
`CycleCommit` e a nova revisão; não existe prefixo publicado com evento sem sua task. Depois, a cadeia
resolve `PerceptionTask` → `Observation` → `KnowledgeInput` → completion respeitando os owners. Cada observation da task tem exatamente seu evento como fonte; task/observation,
instante e observer-recipient coincidem. Claims e evidence do knowledge input são subsets do que a
observation resolvida divulgou. As listas do completion são exatamente as projeções canônicas de
todos os outputs daquela task, sem extras, duplicatas ou omissões; completion vazio representa zero
outputs. Uma observation privada nunca autoriza projetar claims ou evidence de outro ator. A mesma
seção de registries liga `decision-record.rng-draw-refs` somente a `RngDraw` e
`cycle-abort.failure-evidence-refs` somente a `ConflictSet`, `RngDraw`,
`AffordanceAssessment`, `ProvisionalDisposition` ou `AttemptFailure`. O root standalone
`Proposition` e a forma aninhada em `Claim` aplicam ambos o registry `polarity`.

## 5. Containers e ordenação

### 5.1 Ordered list

Lista ordenada é array de definite length e preserva exatamente a ordem do domínio. O codec não
ordena, agrupa nem deduplica. Permutar elementos muda os bytes.

### 5.2 Set-like collection

Coleção set-like também é array, mas seu schema declara:

- uma total ordering key tipada;
- se duplicata byte a byte idêntica é `REJECT` ou `DEDUP_EXACT`;
- a identity key usada para detectar “mesma identidade, bytes diferentes”.

O normalizer calcula/normaliza todos os elementos antes de ordenar. Tuplas de ordenação comparam cada
componente na ordem do schema; text compara UTF-8 NFC, integer por valor e ids/digests por bytes
unsigned. Onde o schema define “canonical bytes da chave”, compara-se lexicograficamente o encoding
CBOR determinístico completo da chave. Mesma identity key com conteúdo diferente sempre falha fechado.

`DEDUP_EXACT` remove somente cópia byte a byte idêntica e apenas nos schemas que o ADR já trata como
set — por exemplo, refs normalizadas. O codec genérico nunca escolhe política de duplicata por conta
própria.

### 5.3 Mapping

Map sem ordem de domínio usa major type 5 e definite length. Chaves V1 são limitadas, conforme schema,
a unsigned integer, byte string ou text string; float, `null`, bool, tag, array e map são proibidos
como chave. ID-keyed mappings com tipo semântico explícito são representados como set-like arrays de
`[id, value]`, não como maps de chaves compostas.

O encoder ordena pares pela ordem bytewise lexicográfica do encoding determinístico completo da chave,
como no RFC 8949 §4.2.1 — **não** usa length-first ordering do RFC 7049 antigo. Chave duplicada é erro;
chave textual duplicada após NFC também.

Record não é map. Usar map de nomes de campo para um record muda o tipo e é inválido mesmo se os
valores forem iguais.

## 6. Subset CBOR aceito

Além das regras acima, V1:

- exige preferred/shortest serialization para integer, length e float;
- proíbe indefinite-length item;
- proíbe todo semantic tag CBOR;
- permite somente simple values `false`, `true` e `null`; `undefined` e simple values custom são
  proibidos;
- rejeita UTF-8 inválido, duplicate map key e container fora da ordem canônica;
- aplica limites de profundidade/tamanho declarados pelo schema/policy antes de alocar;
- rejeita schema, enum, role, domain tag ou versão desconhecidos.

Decoding causal é sempre **strict**: validar CBOR, validar o profile/schema, reemitir os bytes
canônicos e exigir igualdade byte a byte com a entrada. Um decoder permissivo pode existir para UI ou
import, mas sua saída só entra no ledger depois de normalização e strict re-encoding.

## 7. Hash, ids e domain separation

A operação única é:

```text
digest_v1(domain_tag, schema_id, schema_version, value) =
  SHA-256(canonical_bytes_v1(domain_tag, schema_id, schema_version, value))
```

`derive_id` é `digest_v1` sob um domain tag `cote.csf.id.*`; envelope/record/state digests usam
`cote.csf.digest.*` ou `cote.csf.hash.*`. SHA-256 não recebe hex, diagnostic notation, JSON nem texto
intermediário.

O resultado são exatamente os 32 octetos produzidos pelo SHA-256, sem interpretá-los como integer ou
aplicar conversão de endian. Hex só é renderização. A colisão “mesmo id/digest, preimage diferente” é
corrupção: persistência guarda ou reconstrói os bytes canônicos e falha fechado em vez de escolher um
record.

SHA-256 aqui fornece integridade/reprodutibilidade e identidade content-addressed; não autentica o
produtor. Assinatura/MAC, se necessários, são outra política e não alteram os bytes V1.

## 8. Policy e schema bundle

A source of truth independente de linguagem é o diretório
[`canonical-codec-v1-bundle/`](canonical-codec-v1-bundle/README.md), que contém:

- manifesto da policy;
- CDDL por schema/version;
- registry append-only de domain tags;
- registries e field-path bindings de enum/role/variant;
- ordering key, duplicate policy e identity key de toda coleção set-like;
- tabela exaustiva de `reference_kind` para operação/preimage de identidade;
- limites de tamanho/profundidade;
- vetores positivos e negativos da §11;
- versão e checksum do corpus Unicode de conformidade.

O bootstrap do bundle não pode depender do codec que ele próprio define. Por isso:

```text
codec_policy_hash = SHA-256(
  ASCII("cote.csf.bundle.codec-policy.v1") || 0x00 || raw_bytes("policy-manifest.json")
)

schema_bundle_hash = SHA-256(
  ASCII("cote.csf.bundle.schema.v1") || 0x00 || raw_bytes("schema-manifest.json")
)

conformance_suite_hash = SHA-256(
  ASCII("cote.csf.bundle.conformance.v1") || 0x00 || raw_bytes("conformance-manifest.json")
)
```

Cada manifesto lista path ASCII, tamanho e SHA-256 dos bytes exatos de cada artefato, em ordem de
path. `BUNDLE.sha256` publica os três hashes acima e os hashes raw dos manifestos; é recibo e não entra
no preimage. Os valores V1 estão na §1. Item ausente/extra, path duplicado, bytes, tamanho ou hash
divergente falham fechado.

O policy manifest cobre somente `profile.json` e `unicode.json`, pois eles definem as regras globais
que distinguem codec V1 de V2. O schema manifest cobre CDDL e registries; refs de schema no genesis
fixam `(schema_id, schema_version)` sob esse único `schema_bundle_hash`, sem hashes locais sem
derivação. `conformance-manifest.json` cobre `fixtures.json` separadamente. Essa separação elimina o
ciclo que surgiria se um vetor de genesis precisasse carregar o hash de um manifesto que, por sua
vez, incluísse o próprio vetor. Nova versão local muda `schema_bundle_hash`, não `codec_policy_hash`;
mudar a regra global correspondente continua obrigando V2 pela matriz da §10.

Geradores de código são adapters descartáveis. CDDL, registries, política semântica e golden vectors
são autoridade; output gerado ou reflexão de runtime não é.

## 9. Persistência e replay

Todo store causal expõe ao replay a imagem canônica exata do record hashado e sua view tipada. A
topologia física pode armazenar esses bytes diretamente ou reconstruí-los de colunas sem perda, mas a
API de leitura precisa:

1. resolver `codec_policy_id/hash` e schema bundle fixados no genesis;
2. strict-decode/re-encode e comparar os bytes;
3. recomputar id/digest sob o domain tag esperado;
4. comparar refs, ordem, envelope e state hash;
5. somente então chamar reducer.

Biblioteca nova que produz bytes diferentes não “atualiza” o run; ela falha conformidade. Replay sem
a policy/schema/Unicode bundle exata falha fechado. JSON/YAML/database row e CBOR diagnostic notation
são views, não substitutos da imagem canônica.

O hash JSON atual do `Embodiment` permanece um artefato de componente. Só o adapter físico, ainda
aberto, poderá convertê-lo a snapshot/evento causal V1; esta decisão não o reinterpreta retroativamente.

## 10. Evolução e migração

### 10.1 Schema muda; codec não

Adicionar/remover/reordenar campo de record, mudar type/range/optional semantics, código local de
enum/variant/role, limite local ou ordering/duplicate policy de uma coleção cria nova
`schema_version` e novo bundle hash. Alterar uma versão ou entrada existente é proibido. A versão
antiga permanece disponível para replay. Uma run usa somente bundles e versões listados em seu
manifesto imutável; se a nova versão não estava listada no genesis, seu uso exige novo genesis/fork.

### 10.2 Codec muda

Qualquer mudança global em envelope, subset/deterministic rules CBOR, representação de primitivos,
Unicode/repertoire, gramática de identifiers/domain tags, map ordering, float, digest algorithm,
formato de id/digest ou forma textual cria `canonical_bytes_v2` e nova policy. Adicionar um schema ou
domain tag sem mudar essas regras cria bundle novo para nova run, mas preserva o encoder V1. Nunca se
recalculam ids/digests de um histórico V1 in-place.

Migração ocorre por checkpoint verificado e fork:

1. replay V1 até checkpoint e verificar todos os hashes;
2. criar novo `run_id` e gravar no genesis o
   [`parent_checkpoint_history_ref`](cross-policy-checkpoint-reference-v1.md), que carrega policy
   id/hash de origem, domain/schema/version, algoritmo, comprimento e bytes do digest;
3. converter somente o estado de entrada por migrator versionado e com relatório;
4. preservar o prefixo V1 imutável; novos fatos usam ids V2 na nova run.

Um run não troca de codec no meio. Se SHA-256 precisar ser retirado por falha criptográfica, runs V1
ficam read-only até fork explícito; alias ou tabela de tradução não torna um id V1 igual a um id V2.

## 11. Vetores de conformidade

O policy bundle inclui `fixtures.json`, independente de linguagem. JSON é apenas container; não é o
dado hashado. Cada caso positivo contém:

- `case_id`, domain tag, schema id/version;
- payload CBOR canônico validável sob o CDDL root indicado;
- canonical CBOR em lowercase hex;
- SHA-256 em lowercase hex;
- forma textual esperada quando o resultado é id/digest.

Cada caso negativo contém bytes/input e `error_code` estável. Cobertura mínima:

1. `null`, bool e distinção de integer `0/1`;
2. fronteiras 23/24, 255/256, 65535/65536, `u64.max`, `i64.min` e overflow;
3. ordered list cuja permutação muda bytes;
4. todas as permutações de um set/map que convergem aos mesmos bytes;
5. map example do RFC 8949 core ordering e rejeição do length-first antigo;
6. duplicate map key, `DEDUP_EXACT` e identity collision;
7. ASCII, português composto/decomposto, japonês, supplementary plane, invalid UTF-8, surrogate e
   code point unassigned em Unicode 15.1;
8. `+0.0`, `-0.0`, valor que cabe em binary16, apenas binary32, apenas binary64, subnormal e maior
   finito; rejeição de NaN e infinities;
9. enum conhecido/desconhecido, timestamp negativo/positivo e duration;
10. optional absent, present e present-null;
11. raw id/digest com 31/32/33 bytes e rejeição de representação textual dentro do record;
12. mesmo payload sob domain/schema/version diferentes produzindo bytes e digests diferentes;
13. preferred vs integer/length/float superdimensionado, indefinite item, tag e `undefined`;
14. records de todos os artefatos do ADR 0008, inclusive collections e envelopes vazios;
15. binding válido/desconhecido de cada field enum e role registry obrigatório;
16. duplicata exata e colisão de identidade de cada família de ref set-like;
17. idempotência ABSENT/PRESENT, reuso conflitante e recomputação de toda identidade causal;
18. vetores SHA-256 conhecidos do FIPS/NIST e envelopes completos deste profile.

A suíte V1 contém 152 casos positivos — todos os roots persistidos e todas as 73 operações
registradas —, 30 casos negativos com `error_code` estável, 246 casos semânticos de binding,
identidade, ordenação e idempotência, sete casos de normalização e dois vetores SHA-256. O CI de uma
implementação candidata deve executar os mesmos golden vectors em pelo menos duas implementações
independentes e linguagens diferentes como gate de aceitação; este checkpoint arquitetural ainda não
contém esses runners ou workflow. Ambas precisam provar valor
tipado → bytes, bytes → valor tipado estrito,
re-encoding idêntico, digest e os vetores negativos. Comparar somente objetos decodificados não basta.

## 12. Vetores-âncora

Estes vetores pequenos fixam o framing antes da suíte completa. Os hashes foram conferidos por duas
codificações independentes, uma delas uma implementação RFC 8949 externa.

### 12.1 Payload vazio

```text
domain_tag     = cote.csf.test.empty
schema_id      = cote.csf.test.empty
schema_version = 1
value          = []

canonical_hex =
8644435346000173636f74652e6373662e746573742e656d70747973636f74652e6373662e746573742e656d7074790180

sha256 = 0bc3b587d236e3fe4e1589042ae822f881d2f4843eb0cd9f0d770355ac7f6921
```

### 12.2 Primitivos

```text
domain_tag     = cote.csf.test.primitives
schema_id      = cote.csf.test.primitives
schema_version = 1
value          = [null, false, true, 0, -1, 24, "é"]

canonical_hex =
864443534600017818636f74652e6373662e746573742e7072696d6974697665737818636f74652e6373662e746573742e7072696d6974697665730187f6f4f50020181862c3a9

sha256 = 39dc4059e064d7f27686ebccbe7e6c14ade113e11385fae1c92d9baa65c3540a
```

O input `"e\u0301"` normaliza ao mesmo value e precisa produzir exatamente esse vetor.

### 12.3 Domain separation

O mesmo schema/value sob tags diferentes:

```text
schema_id      = cote.csf.test.scalar
schema_version = 1
value          = 1

cote.csf.test.a → 7ba0e959699a7c44448a459e5a4bbe47d26cff209565e807d5a84513a009c848
cote.csf.test.b → 8356b89d399f544040306c93e2695c29c77c2b8c558adb0dd5a6c9903552c7bc
```

Igualdade desses hashes ou reutilização de uma tag para o outro papel é falha de conformidade.
