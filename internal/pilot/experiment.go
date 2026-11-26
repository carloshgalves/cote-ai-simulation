package pilot

import "strings"

// ScenarioZodiacDay2 is a journal-bound operator experiment, not a new
// canonical chronology and not an instruction visible to the characters.
const ScenarioZodiacDay2 = "zodiac-day2"

// Applies equally to every model in this experiment. No mention of the
// calendar, future events, hidden rules, or a preferred strategy.
func ordinaryLifePrompt(prompt string) string {
 prompt=strings.ReplaceAll(prompt,"o que deseja obter, testar, evitar ou descobrir.","o que quer fazer e por quê, inclusive por prazer, hábito, cortesia ou curiosidade comum.")
 prompt=strings.ReplaceAll(prompt,"Considere agir mesmo sem ameaça ou novidade estratégica; se decidir esperar, fundamente em obstáculos concretos.","Pode agir por vontade própria, aproveitar o momento ou esperar sem justificar um obstáculo.")
 prompt=strings.ReplaceAll(prompt,"procure resultados concretos.","não force um resultado estratégico em conversa casual.")
 prompt=strings.ReplaceAll(prompt,"Não aguarde evidência perfeita quando uma experiência reversível puder produzir informação.","Se houver motivo concreto para investigar, pode agir com cautela sem exigir certeza absoluta.")
 return prompt+" Vida cotidiana é um fim legítimo. Uma escola nova, colegas desconhecidos e uma conversa comum não são por si só sinais de ameaça, fraude ou segredo. Não transforme toda interação em sondagem; fale sobre assuntos naturais quando fizer sentido. Uma pessoa desconfiada pode manter sua disposição, mas suspeitas específicas exigem indícios acessíveis. Iniciativa, planejamento, negociação e investigação continuam permitidos quando objetivos e circunstâncias os justificarem."
}
