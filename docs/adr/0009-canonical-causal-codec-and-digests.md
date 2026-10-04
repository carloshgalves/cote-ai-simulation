# ADR 0009 — Codec causal canônico e digests

**Status:** Accepted

**Data:** 2026-09-28

**Relacionado:** ADR 0008 (identidade causal, replay e bytes canônicos), ADR 0007 (Python limitado ao
`Embodiment`)

**Pesquisa:** [codec canônico e digests da fundação causal V1](../research/canonical-causal-codec-v1.md)

**Contrato normativo:** [Canonical Causal Codec V1](../architecture/canonical-codec-v1.md)

**Policy bundle:** [COTE Causal Canonical Codec V1 bundle](../architecture/canonical-codec-v1-bundle/README.md)

## Contexto

O ADR 0008 exige `canonical_bytes_v1`, ids derivados, digests, hashes de estado e verificação byte a
byte, mas deliberadamente não escolheu o wire format nem o algoritmo de digest. Deixar isso para a
primeira implementação faria a linguagem da fundação decidir a semântica do histórico por omissão.
Serialização default de map, enum, float, Unicode ou objeto varia entre runtimes e versões; duas
implementações poderiam concordar sobre o “mesmo objeto” e produzir ids incompatíveis.

A decisão precisa ser adequada a uma fronteira multilíngue. O ADR 0007 não autoriza generalizar
Python para o Simulation Engine causal.

## Decisão

### 1. Perfil binário

Adotar `COTE Causal Canonical Codec V1`, identificado por
`codec_policy_id=cote.csf.codec.cbor-det.v1`:

- RFC 8949 **core deterministic CBOR** como imagem binária;
- envelope fixo `[magic, codec_version, domain_tag, schema_id, schema_version, payload]`;
- CDDL (RFC 8610) e registries versionados como contrato independente de linguagem;
- records como arrays de aridade fixa na ordem do schema;
- maps apenas para semântica de mapping e na ordem lexicográfica dos bytes determinísticos da chave;
- ordered lists preservadas; set-like arrays normalizados pela total ordering/duplicate policy do
  schema;
- semantic tags, indefinite lengths, `undefined`, bignums e chaves complexas proibidos na V1.

O bundle normativo fixa CDDL, domain/schema operations, bindings de cada enum/variant/role,
identidades de `CausalRef`, chaves separadas de ordering/identidade para coleções set-like, limites,
dados Unicode e 436 vetores/casos de conformidade. Seus hashes V1 são:

- `codec_policy_hash=513111dc82a5e58c07aecdabf410633f5aa5418908d2461ef0dff0d9ae5d8203`;
- `schema_bundle_hash=27f1517fdedde2b9fe4561bc950e2420d0f75c2fdbac2ddcfa12913801931aee`;
- `conformance_suite_hash=975a9633ae4649c246966cdfd6cb3b1d87e3c0246aa69cd52b7f50555ce87242`.

O layout completo e os vetores-âncora estão no contrato normativo ligado acima.

### 2. Tipos e texto

- `null`, bool, integer, float, bytes e text permanecem tipos distintos; coerção implícita é inválida;
- integer tem sinal/width/range no schema e usa o menor encoding CBOR do valor;
- float é semanticamente binary64 finito, usa o menor width CBOR que preserve o valor, normaliza
  `-0.0` para `+0.0` e rejeita NaN/infinities;
- valores exatos usam integer coefficient + scale/unidade no schema, nunca float ou decimal tag;
- timestamps são `i64` de microssegundos UTC; enums são códigos unsigned estáveis;
- conteúdo humano usa UTF-8 após NFC pelo Normalization Process for Stabilized Strings (NPSS) sob
  Unicode 15.1.0; unassigned code point, surrogate ou UTF-8 inválido falha fechado;
- todo identificador causal textual usa a gramática ASCII fechada `machine-id` do bundle; NFC/NPSS
  não é política de segurança de identificador e human text não participa de identidade ou ordering;
- ids/digests em records são byte strings de 32 bytes; forma textual é apenas view.

### 3. Digest e domain separation

Todo id/digest V1 é:

```text
SHA-256(canonical_bytes_v1(domain_tag, schema_id, schema_version, value))
```

SHA-256 segue FIPS 180-4 e produz 32 bytes. Cada papel semântico registra domain tag ASCII próprio —
por exemplo, `cote.csf.id.event`, `cote.csf.digest.fence` e `cote.csf.hash.world-state`. Não existe
hash genérico reutilizável entre tipos. Schema id/version também integra o preimage.

A forma textual estrita para diagnóstico é
`csf1:<domain-tag>:sha-256:<64 lowercase hex>`. Ela é decodificada a tag + bytes antes de participar
de qualquer record e nunca é hashada como substituto dos bytes.

### 4. Conformidade e replay

O genesis fixa version/hash do codec, schema bundle, Unicode data e registries. Decoder causal é
strict: valida profile/schema, reencoda e exige os mesmos bytes antes de verificar digest ou aplicar
reducer. Além da forma local, strict validation aplica `record_constraints` e resolve
`typed_value_constraints` valida schema/version externos, operação autorizada, digest dos bytes
exatos e re-encoding canônico do envelope interno. `linked_record_constraints` aplica cada regra na
fronteira de append contra o prefixo durável: retry avança um único attempt desde o último abort do
mesmo run/cycle, seus `rule-versions` coincidem com o novo fence após normalização, e o fence abortado
é reutilizado byte a byte fora dos campos de retentativa; a cadeia epistemológica conserva
event/task, instante, observer-recipient, somente claims/evidências divulgados e a membresia exata da
completion já observável no cursor. Biblioteca nova que emite bytes diferentes é
incompatível com o run, não uma migração.

O policy bundle contém golden vectors positivos/negativos e precisa ser executado por pelo menos duas
implementações independentes e linguagens diferentes. A suíte cobre primitivos, limites numéricos,
Unicode, maps/sets, floats, domain separation, formas CBOR não preferidas e todos os envelopes do ADR
0008.

### 5. Evolução

Mudança local declarada por schema — layout/ordem de campos de record, tipo/range/optional, código de
enum/variant/role, limite local ou ordering/duplicate policy de uma coleção — cria nova
`schema_version` e novo bundle; nunca altera uma versão existente. Mudança global do encoder —
framing, subset/deterministic rules CBOR, representação de primitivos, Unicode/repertoire, gramática
global de identifiers/domain tags, ordem de maps, float, algoritmo/formato de digest ou forma textual
— cria `canonical_bytes_v2` e nova policy. Uma run só usa versões já listadas em seu manifesto
imutável; bundle novo exige novo genesis/fork, ainda que o encoder continue V1.

Um run não troca de codec no meio e histórico V1 nunca é rehashado. Migração verifica checkpoint V1 e
abre novo run/fork cujo genesis carrega o
[`parent_checkpoint_history_ref`](../architecture/cross-policy-checkpoint-reference-v1.md): envelope
estável com policy id/hash de origem, domain/schema/version, algoritmo, comprimento e bytes do digest.
O prefixo antigo permanece imutável e disponível para replay com sua policy original.

## Alternativas descartadas

### RFC 8785 JCS

Tem canonicalização publicada e boa inspeção humana, mas herda o modelo numérico ECMAScript/I-JSON,
não representa bytes nativamente, não normaliza Unicode e exige wrappers próprios para todos os tipos
do ADR. Seria um codec tipado novo escondido dentro de JSON.

### Protocol Buffers determinístico

Excelente para transporte schema-first, mas sua documentação oficial declara que deterministic
serialization não é canonical nem estável para fingerprinting entre builds/versões.

### ASN.1 DER

É canônico e multilíngue, porém exigiria introduzir o modelo ASN.1, tagging e toolchain adicionais.
Para os records já tipados do ADR 0008, core deterministic CBOR + CDDL oferece contrato menor.

### MessagePack ou formato próprio

MessagePack base não fecha as mesmas regras determinísticas. Formato próprio repetiria framing,
números, maps, Unicode, extensões e tooling sem vantagem de domínio.

### SHA3-256 ou BLAKE3-256

Ambos são opções criptográficas válidas; BLAKE3 é mais rápido. SHA-256 vence por disponibilidade
uniforme, padronização e vetores maduros. Envelopes causais pequenos não justificam dependência
adicional por throughput não medido. Domain separation está no preimage e não depende do hash ter API
própria para isso.

## Consequências

- a fundação não pode hashificar JSON/YAML, objetos de runtime ou protobuf diretamente;
- implementações precisam de CBOR strict, CDDL/validação, Unicode 15.1.0 e a suíte golden;
- records binários são menos legíveis; diagnostic notation e JSON/YAML existem apenas como views;
- rejeitar non-finite e unknown schema torna falhas explícitas antes do commit;
- pinning permite replay duradouro, ao custo de manter bundles e codecs antigos;
- esta decisão fecha codec/digest sem escolher linguagem, banco, topologia transacional ou interface
  do adapter físico.
