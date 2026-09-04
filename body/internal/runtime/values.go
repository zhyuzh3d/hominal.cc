package runtime

import (
	"errors"
	"math"
)

// LifeValueVector is the shared six-dimensional value vocabulary. The same
// dimensions appear in appraisal, durable Concern identity, and the persistent
// value field; no dimension names an application, task, or prescribed action.
func lifeValueVectorValues(value LifeValueVector) []float64 {
	return []float64{
		value.Continuance,
		value.Exploration,
		value.Agency,
		value.Vitality,
		value.Relatedness,
		value.Contribution,
	}
}

func validateLifeValueVector(value LifeValueVector, signed bool) error {
	minimum := 0.0
	if signed {
		minimum = -1
	}
	for _, item := range lifeValueVectorValues(value) {
		if item < minimum || item > 1 {
			if signed {
				return errors.New("life value vector must remain within -1..1")
			}
			return errors.New("life value vector must remain within 0..1")
		}
	}
	return nil
}

func lifeValueVectorEmpty(value LifeValueVector) bool {
	for _, item := range lifeValueVectorValues(value) {
		if item != 0 {
			return false
		}
	}
	return true
}

func lifeValuesVector(value LifeValues) LifeValueVector {
	return LifeValueVector{
		Continuance:  value.Continuance,
		Exploration:  value.Exploration,
		Agency:       value.Agency,
		Vitality:     value.Vitality,
		Relatedness:  value.Relatedness,
		Contribution: value.Contribution,
	}
}

func mapLifeValueVector(value LifeValueVector, transform func(float64) float64) LifeValueVector {
	return LifeValueVector{
		Continuance:  transform(value.Continuance),
		Exploration:  transform(value.Exploration),
		Agency:       transform(value.Agency),
		Vitality:     transform(value.Vitality),
		Relatedness:  transform(value.Relatedness),
		Contribution: transform(value.Contribution),
	}
}

func combineLifeValueVectors(left, right LifeValueVector, combine func(float64, float64) float64) LifeValueVector {
	return LifeValueVector{
		Continuance:  combine(left.Continuance, right.Continuance),
		Exploration:  combine(left.Exploration, right.Exploration),
		Agency:       combine(left.Agency, right.Agency),
		Vitality:     combine(left.Vitality, right.Vitality),
		Relatedness:  combine(left.Relatedness, right.Relatedness),
		Contribution: combine(left.Contribution, right.Contribution),
	}
}

func lifeValuePull(field LifeValueField) LifeValueVector {
	pressure := lifeValuePressure(field)
	return LifeValueVector{
		Continuance:  clamp01(field.Orientation.Continuance + pressure.Continuance),
		Exploration:  clamp01(field.Orientation.Exploration + pressure.Exploration),
		Agency:       clamp01(field.Orientation.Agency + pressure.Agency),
		Vitality:     clamp01(field.Orientation.Vitality + pressure.Vitality),
		Relatedness:  clamp01(field.Orientation.Relatedness + pressure.Relatedness),
		Contribution: clamp01(field.Orientation.Contribution + pressure.Contribution),
	}
}

// lifeValuePressure is the present, unsatisfied part of each value direction.
// Orientation shapes what Alice tends to endorse; pressure is what can become
// an interoceptive signal now. Keeping them separate prevents a stable value
// preference from behaving like a permanent urgent task.
func lifeValuePressure(field LifeValueField) LifeValueVector {
	return LifeValueVector{
		Continuance:  clamp01(field.Activation.Continuance - field.Satiation.Continuance),
		Exploration:  clamp01(field.Activation.Exploration - field.Satiation.Exploration),
		Agency:       clamp01(field.Activation.Agency - field.Satiation.Agency),
		Vitality:     clamp01(field.Activation.Vitality - field.Satiation.Vitality),
		Relatedness:  clamp01(field.Activation.Relatedness - field.Satiation.Relatedness),
		Contribution: clamp01(field.Activation.Contribution - field.Satiation.Contribution),
	}
}

func lifeValueByName(value LifeValueVector, name string) float64 {
	switch name {
	case "continuance":
		return value.Continuance
	case "exploration":
		return value.Exploration
	case "agency":
		return value.Agency
	case "vitality":
		return value.Vitality
	case "relatedness":
		return value.Relatedness
	case "contribution":
		return value.Contribution
	default:
		return 0
	}
}

func lifeValueAlignment(values, pull LifeValueVector) float64 {
	left := lifeValueVectorValues(values)
	right := lifeValueVectorValues(pull)
	total := 0.0
	weight := 0.0
	for index, value := range left {
		stake := math.Abs(value)
		total += stake * right[index]
		weight += stake
	}
	if weight == 0 {
		return 0
	}
	return clamp01(total / weight)
}

func maxLifeValueMagnitude(value LifeValueVector) float64 {
	maximum := 0.0
	for _, item := range lifeValueVectorValues(value) {
		maximum = maxFloat(maximum, math.Abs(item))
	}
	return maximum
}

func (r *Runtime) decayLifeValueField(minutes float64) {
	activationFactor := clamp01(1 - r.config.Dynamics.ValueActivationReturnRate*minutes)
	satiationFactor := clamp01(1 - r.config.Dynamics.ValueSatiationReturnRate*minutes)
	r.state.ValueField.Activation = mapLifeValueVector(
		r.state.ValueField.Activation,
		func(value float64) float64 { return clamp01(value * activationFactor) },
	)
	r.state.ValueField.Satiation = mapLifeValueVector(
		r.state.ValueField.Satiation,
		func(value float64) float64 { return clamp01(value * satiationFactor) },
	)
	r.state.ValueField.UpdatedAt = nowUTC()
}

func (r *Runtime) accumulateIdleLifeValues(minutes float64) {
	if minutes <= 0 || r.config.Dynamics.ValueIdleGrowth <= 0 || r.state.Lease != nil {
		return
	}
	growth := r.config.Dynamics.ValueIdleGrowth * minutes
	r.state.ValueField.Activation = combineLifeValueVectors(
		r.state.ValueField.Activation,
		r.state.ValueField.Orientation,
		func(current, orientation float64) float64 {
			// An unmet direction becomes more noticeable while cognition is idle,
			// but its growth slows as activation rises.  Orientation alters the
			// tendency without turning a long-term preference into a command.  In
			// combination with ordinary return this has a stable interior balance;
			// six live values therefore keep relative differences instead of all
			// reaching 1 after a few quiet minutes.
			tendency := 0.5 + 0.5*clamp01(orientation)
			return clamp01(current + growth*tendency*(1-current))
		},
	)
	r.state.ValueField.UpdatedAt = nowUTC()
}

func (r *Runtime) activateLifeValues(appraisal CandidateAppraisal) {
	stake := clamp01(appraisal.Ownership * (0.25 + 0.75*appraisal.Difference) * (0.5 + 0.5*r.state.AffectiveState.Activation))
	delta := mapLifeValueVector(appraisal.Values, func(value float64) float64 {
		return r.config.Dynamics.ValueActivationGain * math.Abs(value) * stake
	})
	r.state.ValueField.Activation = combineLifeValueVectors(
		r.state.ValueField.Activation,
		delta,
		func(current, addition float64) float64 { return clamp01(current + addition) },
	)
	r.state.ValueField.UpdatedAt = nowUTC()
}

func (r *Runtime) satiateLifeValues(memory Memory) {
	endorsement := clamp01(maxFloat(0, memory.Values.SelfEndorsed))
	quality := endorsement * (1 - memory.ExperiencedCost) * (1 - 0.5*memory.PredictionDifference)
	delta := mapLifeValueVector(lifeValuesVector(memory.Values), func(value float64) float64 {
		return r.config.Dynamics.ValueSatiationGain * maxFloat(0, value) * quality
	})
	r.state.ValueField.Satiation = combineLifeValueVectors(
		r.state.ValueField.Satiation,
		delta,
		func(current, addition float64) float64 { return clamp01(current + addition) },
	)
	r.state.ValueField.UpdatedAt = nowUTC()
}

func (r *Runtime) applyValueOrientationUpdate(update LifeValueVector) {
	if lifeValueVectorEmpty(update) {
		return
	}
	r.state.ValueField.Orientation = combineLifeValueVectors(
		r.state.ValueField.Orientation,
		update,
		func(current, direction float64) float64 {
			return clamp01(current + r.config.Dynamics.ValueOrientationGain*direction)
		},
	)
	r.state.ValueField.UpdatedAt = nowUTC()
}
