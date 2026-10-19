# Cross-policy Checkpoint Reference V1

**Status:** Normativo

**Decisão:** [ADR 0009](../adr/0009-canonical-causal-codec-and-digests.md)

Este envelope identifica um checkpoint criado sob uma policy anterior sem depender do codec da run
filha. Ele é framing binário estável e versionado separadamente de `canonical_bytes_vN`.

## Layout

Integers são unsigned big-endian. Textos são ASCII e precisam obedecer às gramáticas abaixo. Nenhum
campo é opcional e nenhum byte trailing é permitido.

| Ordem | Campo | Encoding |
|---:|---|---|
| 1 | magic | 8 bytes `43 53 46 48 52 45 46 00` (`CSFHREF\0`) |
| 2 | reference version | `u8`, valor `1` |
| 3 | origin policy id | `u16` length + ASCII `machine-id` |
| 4 | origin policy hash | `u16` length + bytes; V1 exige length `32` |
| 5 | checkpoint domain tag | `u16` length + ASCII `domain-tag` |
| 6 | checkpoint schema id | `u16` length + ASCII `schema-id` |
| 7 | checkpoint schema version | `u64` |
| 8 | digest algorithm id | `u16` length + ASCII; V1 registra `sha-256` |
| 9 | checkpoint digest | `u16` length + bytes; `sha-256` exige length `32` |

`machine-id`, `domain-tag` e `schema-id` seguem o bundle da policy de origem, que precisa ser obtido
pelo par `(origin policy id, origin policy hash)`. O parser primeiro valida este framing, depois
resolve a policy antiga e só então interpreta/verifica o checkpoint. Algoritmo, comprimento ou schema
não podem ser inferidos da policy filha.

O genesis do fork carrega os bytes completos deste envelope no campo
`parent_checkpoint_history_ref`. Locator de storage, quando necessário, é metadado operacional fora
da identidade; o checkpoint é localizado por seu digest e verificado sob a policy de origem. Incluir
apenas `parent_checkpoint_ref/hash` de 32 bytes é inválido entre policies.

## Vetor V1 → V2

Este vetor usa policy/digest de origem V1, schema de snapshot V1 e 32 bytes `00..1f` para cada hash:

```text
origin_policy_id = cote.csf.codec.cbor-det.v1
origin_policy_hash = 000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f
checkpoint_domain_tag = cote.csf.hash.snapshot
checkpoint_schema_id = cote.csf.schema.snapshot
checkpoint_schema_version = 1
digest_algorithm_id = sha-256
checkpoint_digest = 000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f

reference_hex =
435346485245460001001a636f74652e6373662e636f6465632e63626f722d6465742e76310020000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f0016636f74652e6373662e686173682e736e617073686f740018636f74652e6373662e736368656d612e736e617073686f74000000000000000100077368612d3235360020000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f
```

Decoder deve rejeitar magic/version desconhecido, length inconsistente, algoritmo desconhecido,
ASCII/gramática inválida, hash de policy com tamanho diferente de 32 na V1, digest incompatível com o
algoritmo ou bytes trailing.
