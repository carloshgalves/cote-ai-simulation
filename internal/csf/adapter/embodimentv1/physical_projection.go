package embodimentv1

import (
	"errors"
	"sort"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/reducer"
)

// completedPhysicalBody maps only the closed PSV1-2 BodyState schema after
// independent Go completion validation. Python floats and local sequence
// numbers do not authorize CSF logical order or time.
func completedPhysicalBody(value any) (reducer.PhysicalBody, error) {
	var body reducer.PhysicalBody
	m, ok := value.(map[string]any)
	if !ok {
		return body, ErrInvalidPhysical
	}
	subject, ok := m["character_id"].(string)
	if !ok {
		return body, ErrInvalidPhysical
	}
	body.Subject = subject
	numeric := func(v any) (float64, error) {
		x, valid := number(v)
		if !valid {
			return 0, ErrInvalidPhysical
		}
		return x, nil
	}
	get := func(m map[string]any, key string) (float64, error) { return numeric(m[key]) }
	mapping := func(v any) (map[string]any, error) {
		m, ok := v.(map[string]any)
		if !ok {
			return nil, ErrInvalidPhysical
		}
		return m, nil
	}
	var err error
	if body.Hours, err = get(m, "t_hours"); err != nil {
		return body, err
	}
	w, err := mapping(m["w_prime_balance"])
	if err != nil {
		return body, err
	}
	if body.WPrimeRemaining, err = get(w, "remaining_j"); err != nil {
		return body, err
	}
	if body.WPrimeCapacity, err = get(w, "capacity_j"); err != nil {
		return body, err
	}
	if body.WPrimeTau, err = get(w, "tau_s"); err != nil {
		return body, err
	}
	regions := []string{"legs", "arms", "grip", "core"}
	pf, err := mapping(m["peripheral_fatigue"])
	if err != nil {
		return body, err
	}
	for i, k := range regions {
		if body.Peripheral[i], err = get(pf, k); err != nil {
			return body, err
		}
	}
	if body.CentralFatigue, err = get(m, "central_fatigue"); err != nil {
		return body, err
	}
	load, err := mapping(m["cumulative_load"])
	if err != nil {
		return body, err
	}
	if body.AcuteLoad, err = get(load, "acute_7d"); err != nil {
		return body, err
	}
	if body.ChronicLoad, err = get(load, "chronic_28d"); err != nil {
		return body, err
	}
	sleep, err := mapping(m["sleep"])
	if err != nil {
		return body, err
	}
	if body.SleepDebt, err = get(sleep, "debt_hours"); err != nil {
		return body, err
	}
	if body.HoursAwake, err = get(sleep, "hours_since_wake"); err != nil {
		return body, err
	}
	if body.Circadian, err = get(sleep, "circadian_phase"); err != nil {
		return body, err
	}
	if sleep["last_sleep"] != nil {
		e, err := mapping(sleep["last_sleep"])
		if err != nil {
			return body, err
		}
		s := &reducer.PhysicalSleepEpisode{}
		if s.Start, err = get(e, "start_h"); err != nil {
			return body, err
		}
		if s.End, err = get(e, "end_h"); err != nil {
			return body, err
		}
		s.Quality, ok = e["quality"].(string)
		if !ok {
			return body, ErrInvalidPhysical
		}
		body.LastSleep = s
	}
	energy, err := mapping(m["energy"])
	if err != nil {
		return body, err
	}
	if body.Substrate, err = get(energy, "substrate_availability"); err != nil {
		return body, err
	}
	if body.EnergyBalance, err = get(energy, "balance_kcal_24h"); err != nil {
		return body, err
	}
	if energy["last_meal_at_h"] != nil {
		last, err := get(energy, "last_meal_at_h")
		if err != nil {
			return body, err
		}
		body.LastMeal = &last
	}
	hydration, err := mapping(m["hydration"])
	if err != nil {
		return body, err
	}
	if body.HydrationDeficit, err = get(hydration, "deficit_pct_body_mass"); err != nil {
		return body, err
	}
	thermal, err := mapping(m["thermal"])
	if err != nil {
		return body, err
	}
	if body.CoreOffset, err = get(thermal, "core_offset_c"); err != nil {
		return body, err
	}
	if body.WBGT, err = get(thermal, "wbgt"); err != nil {
		return body, err
	}
	if body.WorkRestRatio, err = get(thermal, "work_rest_ratio"); err != nil {
		return body, err
	}
	if body.ClothingInsulation, err = get(thermal, "clothing_insulation"); err != nil {
		return body, err
	}
	soreness, err := mapping(m["soreness"])
	if err != nil {
		return body, err
	}
	soRegions, err := mapping(soreness["by_region"])
	if err != nil {
		return body, err
	}
	for i, k := range regions {
		so, err := mapping(soRegions[k])
		if err != nil {
			return body, err
		}
		lag, ok := so["latency"].([]any)
		if !ok || len(lag) == 0 || len(lag) > 100 {
			return body, ErrInvalidPhysical
		}
		body.Soreness[i].Latency = make([]float64, len(lag))
		for j, v := range lag {
			if body.Soreness[i].Latency[j], err = numeric(v); err != nil {
				return body, err
			}
		}
		if body.Soreness[i].Expressed, err = get(so, "expressed"); err != nil {
			return body, err
		}
	}
	pain, err := mapping(m["pain"])
	if err != nil {
		return body, err
	}
	painRegions, err := mapping(pain["by_region"])
	if err != nil {
		return body, err
	}
	for i, k := range regions {
		if body.Pain[i], err = get(painRegions, k); err != nil {
			return body, err
		}
	}
	if body.GlobalPain, err = get(pain, "global_intensity"); err != nil {
		return body, err
	}
	if body.Analgesia, err = get(pain, "analgesia"); err != nil {
		return body, err
	}
	_, _, err = reducer.EncodePhysical([]reducer.PhysicalBody{body})
	if err != nil {
		return body, errors.Join(ErrInvalidPhysical, err)
	}
	return body, nil
}

func verifiedCompletedBodies(tips map[string]map[string]any) ([]reducer.PhysicalBody, error) {
	if len(tips) == 0 || len(tips) > 4096 {
		return nil, ErrInvalidPhysical
	}
	ids := make([]string, 0, len(tips))
	for id := range tips {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]reducer.PhysicalBody, 0, len(ids))
	for _, id := range ids {
		body, err := completedPhysicalBody(tips[id]["verified_body"])
		if err != nil || body.Subject != id {
			return nil, ErrInvalidPhysical
		}
		out = append(out, body)
	}
	return out, nil
}

// PhysicalCompletedState is verified, but by itself remains UNADMITTED.
// The canonical copy prevents callers from mutating internal verification
// state and cannot authorize a provenance-independent causal commit.
func (v Verified) PhysicalCompletedState() ([]any, error) {
	if len(v.bodies) == 0 {
		return nil, ErrUnadmitted
	}
	typed, _, err := reducer.EncodePhysical(v.bodies)
	return typed, err
}
