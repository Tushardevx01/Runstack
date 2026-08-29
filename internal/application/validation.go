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

	if err := validateProbe(spec.ReadinessProbe); err != nil {
		return err
	}
	if err := validateProbe(spec.LivenessProbe); err != nil {
		return err
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

func validateProbe(p *Probe) error {
	if p == nil {
		return nil
	}
	if p.Type != "HTTP" && p.Type != "TCP" {
		return errors.New("invalid probe type, must be HTTP or TCP")
	}
	if p.Port <= 0 || p.Port > 65535 {
		return errors.New("invalid probe port")
	}
	if p.Type == "HTTP" && p.Path == "" {
		return errors.New("HTTP probe requires a path")
	}
	if p.InitialDelaySecs < 0 {
		return errors.New("probe initial delay cannot be negative")
	}
	if p.PeriodSecs <= 0 {
		return errors.New("probe period must be > 0")
	}
	if p.TimeoutSecs <= 0 {
		return errors.New("probe timeout must be > 0")
	}
	if p.SuccessThreshold <= 0 {
		return errors.New("probe success threshold must be > 0")
	}
	if p.FailureThreshold <= 0 {
		return errors.New("probe failure threshold must be > 0")
	}
	return nil
}
