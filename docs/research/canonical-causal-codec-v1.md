# Pesquisa — codec canônico e digests da fundação causal V1

**Data da decisão:** 2026-09-28

**Pergunta:** qual representação produz os mesmos bytes e digests em implementações potencialmente
multilíngues, sem depender da serialização default de uma runtime?

**Decisão informada:** [ADR 0009](../adr/0009-canonical-causal-codec-and-digests.md)

**Contrato resultante:** [Canonical Causal Codec V1](../architecture/canonical-codec-v1.md)

## 1. Restrições do repositório

O codec precisa satisfazer simultaneamente:

- os ids e digests do ADR 0008 são parte da semântica causal, não checksums de conveniência;
- record, map, sequência ordenada e coleção set-like têm semânticas diferentes;
- replay precisa rejeitar deriva de bytes antes de chamar reducer;
- timestamps são microssegundos inteiros, e ids/digests atravessam a fronteira como bytes;
- a stack da fundação ainda está aberta; ADR 0007 escolhe Python somente para `Embodiment`;
- o corpus usa texto japonês e português, portanto “ASCII only” não é opção;
- a integração física futura traz números de ponto flutuante, embora o core causal deva preferir
  inteiros ou fixed-point quando a regra exigir aritmética exata;
- versão/hash do codec, schemas e políticas precisam permanecer disponíveis durante toda a vida do
  run.

## 2. Fatos verificados

Fontes consultadas em 2026-09-28:

- [RFC 8949](https://www.rfc-editor.org/rfc/rfc8949.html), Standards Track, define CBOR e seus
  requisitos de core deterministic encoding: representação preferida e mais curta, containers de
  tamanho definido e maps ordenados lexicograficamente pelos bytes determinísticos das chaves. O
  modelo distingue integer, float, bytes e text.
- [RFC 8610](https://www.rfc-editor.org/rfc/rfc8610.html) define CDDL como notação legível e
  processável para schemas CBOR/JSON e permite validação automática da forma.
- [RFC 8785](https://www.rfc-editor.org/rfc/rfc8785.html) define JCS sobre o subset I-JSON. Números
  seguem a serialização ECMAScript/IEEE-754 binary64, propriedades são ordenadas como unidades UTF-16
  e strings são preservadas sem normalização Unicode.
- A documentação oficial do Protocol Buffers afirma explicitamente que
  [serialização determinística não é serialização canônica](https://protobuf.dev/programming-guides/serialization-not-canonical/)
  e que bytes podem variar com schema, build ou biblioteca.
- [ITU-T X.690](https://www.itu.int/rec/T-REC-X.690-202102-I) mantém DER/CER/BER como regras de
  encoding ASN.1; DER oferece uma forma única, mas exige o modelo e toolchain ASN.1.
- [FIPS 180-4](https://csrc.nist.gov/pubs/fips/180-4/upd1/final) especifica SHA-256 com digest de
  256 bits. A família SHA-2 permanece listada pelo NIST entre os algoritmos aprovados.
- [FIPS 202](https://csrc.nist.gov/pubs/fips/202/final) especifica SHA3-256, entre outros hashes.
- BLAKE3 possui [especificação e implementações oficiais](https://github.com/BLAKE3-team/BLAKE3-specs),
  com vantagem declarada de throughput e paralelismo. Diferentemente de SHA-256, não é definido por
  um FIPS usado nesta comparação.
- [UAX #15](https://www.unicode.org/reports/tr15/) define NFC e o Normalization Process for
  Stabilized Strings (NPSS). NPSS rejeita code points não atribuídos na versão fixada e garante que
  a string normalizada permaneça estável sob versões passadas ou futuras. A versão Unicode 15.1.0 e
  seu [NormalizationTest.txt](https://www.unicode.org/Public/15.1.0/ucd/NormalizationTest.txt) são
  artefatos versionados.

## 3. Alternativas de serialização

### 3.1 RFC 8949 core deterministic CBOR + perfil da aplicação

**Pontos fortes verificados:** formato Standards Track, binário e tipado; distingue bytes/text e
integer/float; possui ordem normativa de map, formas preferidas e comprimentos definidos; há
implementações em várias linguagens. CDDL oferece uma descrição de schema independente da runtime.

**Lacunas que o perfil precisa fechar:** CBOR genérico ainda permite tags, `undefined`, non-finite,
strings não normalizadas, chaves complexas e escolhas de modelo numérico. RFC 8949 deliberadamente
deixa o protocolo da aplicação escolher essas restrições.

**Avaliação:** melhor base, desde que “usar CBOR” não seja confundido com aceitar qualquer CBOR. O
perfil do projeto precisa ser estrito e acompanhado por vetores negativos.

### 3.2 RFC 8785 JSON Canonicalization Scheme

**Pontos fortes verificados:** texto inspecionável, ecossistema amplo, algoritmo publicado e vetores
existentes. É adequado quando o modelo de dados já é I-JSON.

**Custos para este domínio:** não representa bytes nativamente; limita todos os números ao modelo
binary64 ou exige wrappers textuais; não normaliza Unicode; ordena propriedades por UTF-16, embora os
bytes finais sejam UTF-8; não distingue record, enum, timestamp, id ou integer width sem convenções
extras. Essas convenções formariam um segundo codec tipado sobre JSON.

**Avaliação:** rejeitado como imagem hashável. JSON continua válido como view de diagnóstico, nunca
como fonte dos bytes causais.

### 3.3 Protocol Buffers determinístico

**Pontos fortes verificados:** schema-first, code generation e interoperabilidade excelentes para
mensageria.

**Custo decisivo:** o próprio projeto declara que deterministic serialization não é canonical e não
é estável para fingerprinting entre builds, versões e mudanças de schema.

**Avaliação:** rejeitado para preimage de ids/digests. Continua possível como transporte externo que
é validado e convertido ao modelo causal antes da admissão.

### 3.4 ASN.1 DER

**Pontos fortes verificados:** encoding distinguido maduro e único, schema formal, implementações em
múltiplas linguagens.

**Custos:** adiciona ASN.1, tagging e toolchain próprios; schemas do projeto teriam de ser expressos
novamente nesse modelo; SET/SEQUENCE e regras de extensibilidade são mais complexos do que a pequena
superfície necessária. O ganho de maturidade não compensa a distância do modelo já descrito pelo ADR
0008.

**Avaliação:** tecnicamente válido, mas rejeitado por complexidade e custo de evolução.

### 3.5 MessagePack ou formato binário próprio

MessagePack não oferece no padrão base um perfil determinístico tão completo quanto RFC 8949. Um
formato próprio teria de especificar novamente framing, números, mapas, UTF-8, extensões e tooling.

**Avaliação:** rejeitados; ambos recriariam trabalho já padronizado.

## 4. Alternativas de digest

| Algoritmo | Vantagem | Custo para a fundação |
|---|---|---|
| SHA-256 | FIPS 180-4, ubíquo, vetores e implementações maduras, 32 bytes | menor throughput que BLAKE3; irrelevante para envelopes pequenos nesta V1 |
| SHA3-256 | FIPS 202 e construção diferente de SHA-2 | suporte em standard libraries é menos uniforme; nenhum requisito do domínio pede diversidade criptográfica |
| BLAKE3-256 | muito rápido, paralelo e com domain separation própria | dependência adicional em várias stacks; especificação fora do baseline NIST atual |

**Recomendação:** SHA-256. O codec faz domain separation no preimage; não precisa escolher hash mais
novo para obter essa propriedade. Desempenho do hash não é gargalo demonstrado, e ubiquidade reduz o
risco de duas implementações dependerem de bindings diferentes.

## 5. Unicode

### Fato

NFC é estável para caracteres atribuídos, e NPSS transforma diferença de versão em rejeição explícita
quando o texto contém code point não atribuído no repertoire fixado. Isso é melhor para causalidade do
que duas runtimes normalizarem silenciosamente de modos distintos.

### Inferência arquitetural

[Unicode 18.0](https://www.unicode.org/versions/Unicode18.0.0/) foi publicado em 2026-09-16, doze dias
antes desta decisão. A V1 não precisa de seu novo repertoire para ids, nomes de schema ou texto atual
do cenário. Fixar Unicode 15.1.0 como baseline conservador reduz a exigência mínima sobre runtimes
candidatas; uma implementação ainda pode vendorizar os dados normativos em vez de confiar na versão
do host. Expandir o repertoire será mudança de política, não atualização invisível de biblioteca.

**Recomendação:** NFC por NPSS sob Unicode 15.1.0; rejeitar unassigned code points, surrogate isolado,
UTF-8 inválido e chave duplicada depois da normalização.

## 6. Recomendação final

Adotar `COTE Causal Canonical Codec V1`:

- RFC 8949 core deterministic CBOR como encoding binário;
- envelope tipado com magic, versão do codec, domain tag, schema id/version e payload;
- records como arrays na ordem do schema; CDDL como fonte de forma;
- profile fechado para primitivos, maps, sequências e sets;
- NFC/NPSS Unicode 15.1.0;
- floats finitos apenas, `-0.0` normalizado, sem coerção integer/float;
- SHA-256 de 32 bytes;
- ids e digests como bytes dentro de records; forma textual só para views;
- domain tag distinto por papel semântico;
- policy/schema bundle e vetores golden hashados e fixados no genesis;
- replay estrito com codec antigo; mudança de codec exige fork, nunca rehash in-place.

O contrato completo está em
[`docs/architecture/canonical-codec-v1.md`](../architecture/canonical-codec-v1.md). A escolha é
independente da linguagem da fundação e não decide sua stack.
