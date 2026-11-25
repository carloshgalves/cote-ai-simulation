package pilot

// decisionNeedsModel is the single versioned routing predicate used by live
// execution, fast-forward and journal validation. Resolution describes the
// actor's representation, not an obligation to query a provider.
func decisionNeedsModel(j Journal, s State, id string) bool {
 if j.Version < SelectiveVersion {
  return needsModel(s, id)
 }
 m := s.Minds[id]
 if m.Activity != nil || (m.Tier != FullTier && m.Tier != LightTier) {
  return false
 }
 if j.Version>=StaffCareVersion&&!isStudent(j,id)&&staffUrgentCare(j,s,id)&&m.Conversation==nil{return false}
 if m.PendingStimulus != "" || (m.Conversation != nil && m.Conversation.Turn == id) {
  return true
 }
 if m.NextDeliberation > 0 {
  return s.Minute >= m.NextDeliberation
 }
 // Only participants can deliberate about the active exam. A deliberate
 // wait for an event remains respected until a new addressed stimulus.
 if s.World != nil && s.World.Exam != nil {
  e := s.World.Exam
  if has(e.Group, id) && (e.Phase == "early" || e.Phase == "final") && !e.Submitted[id] && !m.AwaitEvent {
   return true
  }
 }
 // A bounded endogenous opportunity preserves initiative in ordinary days.
 // It is not a mandatory decision every time a routine task ends.
 return initiativeDue(j,s,id)
}

// An admission-time recognition is an actor's own biographical fact plus
// co-presence, not a grant of another actor's private background.
func initializeSelectiveStimuli(j Journal, s *State) {
 for _, id := range ids(j.Profiles) {
  m := s.Minds[id]
  for _, other := range ids(j.Profiles) {
   if id != other && m.Location == s.Minds[other].Location && recognizes(j, id, other) {
    stimulate(s, id, "Presença de pessoa conhecida antes da matrícula")
    break
   }
  }
 }
}
