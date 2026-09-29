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
4. `unicode.json`, para a versão e os dados Unicode exatos;
5. `fixtures.json`, para bytes/digests positivos e códigos de erro negativos;
6. `policy-manifest.json` e `schema-manifest.json`, para a composição e os hashes do bundle.

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
```

Os manifestos são arrays JSON ordenados por `path`; cada entrada fixa `path`, `size` e
`sha256_lower_hex`. O verificador precisa rejeitar path repetido, item ausente/extra, tamanho ou hash
divergente e bytes JSON diferentes dos commitados. Os dois hashes resultantes estão em
`BUNDLE.sha256`; esse arquivo é um recibo e não entra em nenhum dos dois hashes.

`policy-manifest.json` contém somente `profile.json` e `unicode.json`: regras globais cuja mudança
cria codec V2. `schema-manifest.json` contém `foundation.cddl`, `registries.json` e `fixtures.json`:
regras locais versionáveis sem mudar o encoder. Assim um novo schema/domain tag altera apenas um
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
externo ser aceito.

Uma extensão pode adicionar schema/domain tag, enum ou role sem alterar este diretório, mas não pode
redefinir nome/código existente nem relaxar o profile. O genesis fixa o hash de cada extension bundle
e o conjunto de schema versions admitidas. Uma run não incorpora bundle publicado depois do genesis.
`causal-ref` e `evidence-ref` usam os códigos de `reference_kind`; `input-ref` usa `unit_kind`.
Roles/códigos extension-specific só são válidos quando o extension bundle requerido pelo registry os
declara de forma append-only.

`enum_bindings` e `role_bindings` ligam cada field path ao registry aplicável; o tipo base
`u8`/`u16` sozinho nunca autoriza um código. `reference_identities` liga todos os 23
`reference_kind` à operação/schema de id e aos componentes persistidos do preimage. Cada entrada de
`set_like_collections` tem quatro posições normativas — field path, ordering key, duplicate policy e
identity key —; mesma identity key com bytes diferentes é sempre `SET_IDENTITY_COLLISION`.

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

`fixtures.json` contém 141 vetores positivos, 28 negativos, 95 casos semânticos, cinco casos de
normalização convergente e dois vetores SHA-256. Os positivos cobrem primitivos e limites, Unicode,
map/list/set, floats, todos os roots persistidos e as 65 operações de id/digest/hash do registry.
Cada um fixa payload CBOR, envelope completo e SHA-256. Os negativos fixam bytes/input e
`error_code` estável. Os casos semânticos executam bindings de enum, duplicata/colisão de cada família
de ref, dois attempts do mesmo ciclo, permutação de `rule-versions` e idempotência
ABSENT/PRESENT/conflitante.

No checkpoint documental, os 141 vetores positivos foram produzidos por um encoder isolado em
Node.js 22.22.1 e, independentemente, decoded, reencoded byte a byte e rehashados por um segundo
encoder/decoder em Python 3.14.4; o segundo verificador também confirmou os cinco casos de
normalização e os dois vetores SHA-256. Esses programas foram ferramentas temporárias, não
implementação da fundação nem parte do bundle. A aceitação de uma implementação continua exigindo que
ela execute positivos e negativos e faça validação CDDL/semântica, conforme o contrato principal.

## Classificação de texto

Todo campo textual do schema é exatamente uma destas classes:

- `machine-id`: identidade, namespace, role, reason code, policy/schema/domain id ou chave causal;
  ASCII minúsculo, 1–128 octetos, sob a ABNF integral de `profile.json` — inicia por `[a-z]` e cada
  separador em `._:/-` é seguido por um ou mais `[a-z0-9]`;
- `human-text`: conteúdo humano; Unicode scalar values, NFC pelo Normalization Process for
  Stabilized Strings (NPSS) de Unicode 15.1.0, sem code point unassigned nessa versão;
- `ascii-uri`: locator não autoritativo, ASCII visível sob a gramática registrada no schema.

O CDDL usa aliases `tstr` para essas classes e delega a validação lexical ao ABNF de `profile.json`.
Isso é intencional: `.regexp` do RFC 8610 usa a linguagem de regex XML Schema, portanto padrões em
sintaxe PCRE/ECMAScript seriam não portáveis. Validar somente o tipo CDDL, sem aplicar o profile e os
dados Unicode, é não conforme.

`machine-id` não aceita maiúscula, espaço, controle, bidi, default-ignorable ou qualquer non-ASCII;
logo não depende de detecção de script/confusable. `human-text` nunca pode ser domain tag, schema id,
map key, identity key ou ordering key. NFC/NPSS estabiliza bytes de conteúdo; não é apresentado como
controle de spoofing de identificadores.

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
