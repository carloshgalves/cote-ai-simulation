# Examination architecture

Cada exame especial existente deve virar uma especificação executável. O LLM não decide o vencedor.

## ExamSpec mínimo

- `exam_id` e versão;
- participantes e agrupamentos;
- período e deadlines;
- recursos iniciais;
- regras públicas;
- regras/roles secretos;
- canais de informação;
- ações válidas;
- validações e efeitos;
- scoring/pontuação;
- condições de expulsão/transferência quando aplicável;
- eventos disparados por tempo;
- seed para sorteios reproduzíveis.

## Fluxo

1. engine cria o estado do exame;
2. disclosure system entrega a cada agente apenas regras/informações autorizadas;
3. agentes propõem ações;
4. validator rejeita ações inválidas;
5. resolver aplica efeitos ao world state;
6. event log registra tudo;
7. scoring calcula resultado independentemente de narrativa.

## Vertical slice recomendado

Começar com um único exame que tenha informação privada, cooperação e estratégia. Isso exercita simultaneamente regras, memória, conhecimento parcial e multiagente antes de escalarmos para o calendário inteiro.
