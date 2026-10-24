# Atualização de objetivos baseada em evidências — cânone de personagens

**Escopo:** dados canônicos descritivos. **Não** é um mecanismo executável de atualização de objetivos. Referências: ADR-0012 (revelações cumulativas), políticas de memória e verdade/crença existentes.

## Problema

Uma ficha de personagem que armazena apenas um "objetivo inicial" congela a personalidade e compromete a simulação. Um objetivo pode permanecer enquanto o meio muda; prioridades, alvo, incentivos e horizonte temporal podem mudar; um compromisso pode não se concretizar. Evidências literárias ilustram **como e por que uma mudança ocorreu**, não determinam o futuro.

## Representação

- `goal_transitions/y1-v05.json` registra `prior_goal_or_means`, `observed_goal_or_means_after`, `kind`, `trigger_or_new_information`, `behavioral_evidence`, `event_time_ref` e `source.pdf_pages`.
- As modalidades são **adição de objetivo**, **alteração de meio**, **reordenação de prioridades**, **mudança de alvo**, **reavaliação de fracasso** e **interrupção de plano**. Não usar uma única etiqueta "cresceu" para todas.
- `epistemic_status` distingue pensamento narrado, declaração de intenção, promessa, comportamento observável e inferência de terceiros; `persistence_scope` declara se a durabilidade está comprovada.
- `contingent_alternatives` descreve condições que **podem** levar a caminhos diferentes; não define scripts inevitáveis.
- `simulation_guard` impede impor romance recíproco, perdão, vitória, traição ou estabilidade emocional sem ações correspondentes na linha temporal simulada.
- `indexes/goal-transition-map.json` possibilita busca por personagem e evidência; os packs globais apontam para os IDs, nunca substituem o histórico dos volumes anteriores.

## Exemplo: Horikita

O objetivo mais distante é chegar à Classe A e obter reconhecimento de Manabu. No Volume 5, ela mantém esse objetivo, mas passa a **valorizar cooperação como meio** após falhar sozinha e reconhecer Sudō como aliado. É incorreto modelar isso como desistência de Classe A, abandono do irmão ou adesão irreversível a toda proposta de grupo.

## Exemplo: Sudō

Sudō demonstra afeto romântico unilateral por Suzune, busca reconhecimento além do basquete, aprende com a própria falha e aceita uma restrição verbal contra violência injustificada. Nada no capítulo prova sentimento recíproco de Suzune ou cumprimento vitalício da promessa.

## Exemplo: Kushida e Ryūen

Kushida declara que expulsar Horikita, depois também Ayanokōji, importa mais do que subir de classe. Ryūen revela prioridade de humilhação de Suzune sobre pontos e posteriormente interrompe a pressão ao receber gravação anônima. Metas públicas, privadas, intenções declaradas e execução devem ocupar campos distintos; a gravação não torna seu remetente conhecido para Ryūen.

## Exemplo: Ayanokōji

Ayanokōji diz querer que Horikita identifique a pessoa infiltrada e não modifica imediatamente a escalação. Ele usa perda potencial como oportunidade de aprendizado; o texto não prova que tenha causado **deliberadamente todos os resultados negativos** nem autoriza generalizar que sempre sacrifica exames por desenvolvimento.

## Contrato futuro de execução (proposta, não implementado)

1. Um agente começa com metas e crenças vigentes apenas para aquele momento. Nunca receber automaticamente segredos futuros do cânone.
2. Cada decisão avalia objetivos presentes, novas informações, relações, recursos, riscos e consequências. Um evento pode reforçar, acrescentar, enfraquecer ou substituir metas; também pode não produzir mudança.
3. A transição precisa de justificativa observável no histórico da simulação e de atualização explícita do estado do ator; identidade, memória e crença devem continuar separadas da verdade do mundo.
4. Permitir manutenção, reversão, regressão, conflito entre metas e incerteza, sem pesos numéricos literários inventados.
5. Rodar testes de controle negativo: mudança automática após evento que o ator não testemunhou, transformação garantida após um pedido de desculpas, exposição indevida de gravação anônima, romance recíproco forçado e objetivo de "perder deliberadamente" atribuído sem evidência.
6. Nunca promover dados canônicos diretamente para execução sem review independente, integração própria ao planejador, testes e gate humano.

## Estado

Os eventos do Volume 5 documentam **exemplos históricos pesquisáveis** da flexibilidade das metas, não um motor de simulação de desejos. `source-review-register-y1-v05.json` mantém revisão independente e aprovação de liberação pendentes.
