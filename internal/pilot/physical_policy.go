package pilot

// Admission-safe action constraint, not a capacity score. Later evidence
// supports a provisional restriction, not an imported episodic memory.
type PilotPhysicalPolicy struct{
 VigorousExerciseRestricted bool `json:"vigorous_exercise_restricted,omitempty"`
 Evidence string `json:"evidence"`
 Status string `json:"status"`
 Description string `json:"description"`
}
func physicalPolicyFor(id string)*PilotPhysicalPolicy{
 switch id{
 case "arisu-sakayanagi":
  return &PilotPhysicalPolicy{true,"y1v5.case.arisu-sakayanagi.maintain-faction-support-withdraw-athletics; y1v5.ev.010","PROVISIONAL_ADMISSION_RESTRICTION","Evitar atividade esportiva vigorosa; mobilidade cotidiana e atividades leves continuam possíveis."}
 case "ken-sudo":
  return &PilotPhysicalPolicy{false,"y1v5.case.ken-sudo.practice-coaching-and-temper; y1v5.ev.019","PROVISIONAL_DESCRIPTIVE","Aptidão esportiva não implica esforço máximo, superioridade universal ou imunidade à fadiga."}
 }
 return nil
}
func allowsVigorousExercise(j Journal,id string)bool{
 p:=j.Identities[id].Physical
 return p==nil||!p.VigorousExerciseRestricted
}
