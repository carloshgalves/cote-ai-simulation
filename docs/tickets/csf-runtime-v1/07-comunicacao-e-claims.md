# CSFRV1-7 — Comunicação, claims e entrega causal

**Spec:** §7.7 (claims/comunicação) · §8.1–8.3 · §9 (transmission)  
**ADR 0008:** invariantes 9, 11 e exercício de 10/41  
**Bloqueado por:** CSFRV1-6  
**Bloqueia:** CSFRV1-8

## Resultado observável

A envia a B a alegação falsa de que C roubou um recurso, apresentando C como remetente, com entrega
agendada. Antes do due time B não recebe nada; depois do evento de entrega B recebe um claim com
`presented_sender=C`, sem `actual_sender=A` e sem surgir `resource.stolen` no mundo. Falha de canal
produz fato de falha e nenhum input.

## Escopo

- `Proposition`, `Claim` sem truth flag e `Transmission` com sender real/apresentado separados.
- Handlers/reducers para `communication.sent`, delivery/failure agendados e lifecycle in-transit.
- Política de canal/observer projeta conteúdo e identidade apresentada sem oracle do caller.
- Entrega cria evento posterior; percepção normal do CSFRV1-6 produz evidence/input.
- Comparar claim com world truth, quando necessário, exige regra explícita e novo fato, nunca mutação
  silenciosa do claim.

## Arquivos e módulos prováveis

```text
internal/csf/{communication,claim}/
internal/csf/extensions/communicationv1/
internal/csf/{communication,claim}/**/*_test.go
tests/scenarios/{false_claim,forged_sender,delayed_delivery}_test.go
evals/knowledge-boundary/csf-runtime-v1/communication.*
```

## Testes determinísticos

- Envio cria transmission `IN_TRANSIT`; não cria input do destinatário.
- Pausa antes do due time e resume posterior disparam exatamente uma entrega.
- Delivery e failure são mutuamente exclusivos e terminam a transmission.
- Claim falso chega como alegação; não cria o evento alegado nem altera world truth.
- Destinatário vê apenas presented sender quando policy assim define; actual sender segue world truth.
- Mensagem não observada/delivery failure produz zero `KnowledgeInput`.
- Atraso semântico usa occurrence/event posterior; fila operacional nunca muda timestamps causais.

## Property tests

Permutar processamento de canais/transmissions independentes preserva ids, agenda, eventos e
evidência. Retry de delivery não duplica claim/input. Alterar latência operacional sem alterar
occurrences não muda bytes autoritativos.

## Evals e checagens de conhecimento

Executar `knowledge-boundary-audit` sobre falsidade e remetente falsificado. Verificar que truth
global, actual sender, transmission interna e observações de terceiros não aparecem no payload de B.
Sem eval de modelo: B não forma crença nesta V1.

## Fora do escopo

Leitura/atenção, belief update, rumor/reflexão, reputação, punição e consequência social.

## Evidência de conclusão

1. Timeline persistida distingue send, due delivery e receipt.
2. Evidence de B contém claim/presented sender e omite actual sender.
3. Event store não contém `resource.stolen` no cenário falso.
4. Replay parcial da transmissão não duplica entrega nem input.
