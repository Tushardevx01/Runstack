package application

import "errors"

func ValidateAppSpec(spec AppSpec) error {
	if spec.Replicas < 0 {
		return errors.New("replicas cannot be negative")
	}

	if spec.Strategy != nil && spec.Strategy.Type == RolloutStrategyRollingUpdate {
		if spec.Replicas >= 1 && spec.Strategy.MaxSurge == 0 && spec.Strategy.MaxUnavailable == 0 {
			return errors.New("MaxSurge and MaxUnavailable cannot both be 0 when replicas >= 1 (deadlock risk)")
		}
	}

	return nil
}

func EnsureDefaultStrategy(spec *AppSpec) {
	if spec.Strategy == nil {
		spec.Strategy = &RolloutStrategy{
			Type:           RolloutStrategyRollingUpdate,
			MaxSurge:       1, // default
			MaxUnavailable: 1, // default
		}
	} else if spec.Strategy.Type == RolloutStrategyRollingUpdate {
		// apply 25% logic if they are 0 and replicas > 1?
		// The design says "If left empty, default to MaxSurge = 25% (min 1)".
		// But they passed a strategy, so if they passed surge=0 and unavailable=1, it's valid.
	}
}
