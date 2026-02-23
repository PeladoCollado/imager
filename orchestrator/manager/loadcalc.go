package manager

import "math"

type LoadCalculator interface {
	Next() int
}

type FeedbackLoadCalculator interface {
	LoadCalculator
	Observe(observation LoadObservation)
}

type LoadObservation struct {
	RoundID           string
	TotalRPS          int
	PlannedRequests   int
	CompletedRequests int
	SuccessCount      int
	FailureCount      int
	TimeoutCount      int
	P99LatencyMillis  int64
}

func (l LoadObservation) TimeoutRatio() float64 {
	denominator := l.CompletedRequests
	if denominator <= 0 {
		denominator = l.PlannedRequests
	}
	if denominator <= 0 {
		return 0
	}
	return float64(l.TimeoutCount) / float64(denominator)
}

func NewStepFunctionLoadCalculator(minRps int, maxRps int, stepSize int) LoadCalculator {
	return &StepFunctionLoadCalculator{minRps: minRps, maxRps: maxRps, stepSize: stepSize, currRps: minRps}
}

func NewExponentialLoadCalculator(minRps int, maxRps int) LoadCalculator {
	return &ExponentialFunctionLoadCalculator{minRps: minRps, maxRps: maxRps, factor: 2, currRps: minRps}
}

func NewLogarithmicLoadCalculator(minRps int, maxRps int) LoadCalculator {
	return &ExponentialFunctionLoadCalculator{minRps: minRps, maxRps: maxRps, factor: 10, currRps: minRps}
}

func NewAdaptiveExponentialLoadCalculator(minRps int, maxRps int, maxLatencyMillis int64) LoadCalculator {
	if minRps < 0 {
		minRps = 0
	}
	if maxRps < 0 {
		maxRps = 0
	}
	if maxRps < minRps {
		minRps = maxRps
	}
	recoveryRps := 1
	if maxRps < recoveryRps {
		recoveryRps = maxRps
	}
	return &AdaptiveExponentialLoadCalculator{
		minRps:                 minRps,
		maxRps:                 maxRps,
		maxLatencyMillis:       maxLatencyMillis,
		recoveryRps:            recoveryRps,
		minBinaryGranularity:   10,
		phase:                  adaptivePhaseRamp,
		nextRps:                minRps,
		lowestUnsuccessfulRps:  -1,
		highestSuccessfulRps:   0,
		highestSuccessfulKnown: false,
	}
}

type StepFunctionLoadCalculator struct {
	minRps   int
	maxRps   int
	stepSize int

	currRps int
}

func (s *StepFunctionLoadCalculator) Next() int {
	n := s.currRps
	if s.currRps+s.stepSize > s.maxRps {
		s.currRps = s.maxRps
	} else {
		s.currRps += s.stepSize
	}
	return n
}

type ExponentialFunctionLoadCalculator struct {
	minRps  int
	maxRps  int
	factor  int
	currRps int
}

func (e *ExponentialFunctionLoadCalculator) Next() int {
	n := e.currRps
	if e.currRps*e.factor > e.maxRps {
		e.currRps = e.maxRps
	} else {
		e.currRps *= e.factor
	}
	return n
}

type adaptivePhase string

const (
	adaptivePhaseRamp   adaptivePhase = "ramp"
	adaptivePhaseSearch adaptivePhase = "search"
	adaptivePhaseSteady adaptivePhase = "steady"
)

type AdaptiveExponentialLoadCalculator struct {
	minRps           int
	maxRps           int
	maxLatencyMillis int64
	recoveryRps      int

	minBinaryGranularity int
	phase                adaptivePhase
	awaitingRecovery     bool
	pendingSettle        bool

	nextRps                int
	highestSuccessfulRps   int
	highestSuccessfulKnown bool
	lowestUnsuccessfulRps  int
}

func (a *AdaptiveExponentialLoadCalculator) Next() int {
	return a.nextRps
}

func (a *AdaptiveExponentialLoadCalculator) Observe(observation LoadObservation) {
	failed := a.thresholdExceeded(observation)

	if a.awaitingRecovery {
		if failed {
			a.nextRps = a.recoveryRps
			return
		}
		a.recordSuccess(observation.TotalRPS)
		a.awaitingRecovery = false
		if a.phase == adaptivePhaseSearch {
			if a.pendingSettle {
				a.pendingSettle = false
				a.phase = adaptivePhaseSteady
				a.nextRps = a.bestSustainableRps()
				return
			}
			nextProbe, ok := a.nextBinaryProbeRps()
			if !ok {
				a.phase = adaptivePhaseSteady
				a.nextRps = a.bestSustainableRps()
				return
			}
			a.nextRps = nextProbe
			return
		}
		a.nextRps = a.clampRps(a.nextRps)
		return
	}

	switch a.phase {
	case adaptivePhaseRamp:
		if failed {
			a.recordFailure(observation.TotalRPS)
			a.phase = adaptivePhaseSearch
			a.awaitingRecovery = true
			a.nextRps = a.recoveryRps
			return
		}
		a.recordSuccess(observation.TotalRPS)
		a.nextRps = a.nextRampRps(observation.TotalRPS)
	case adaptivePhaseSearch:
		if failed {
			a.recordFailure(observation.TotalRPS)
		} else {
			a.recordSuccess(observation.TotalRPS)
		}
		a.pendingSettle = a.searchConverged()
		a.awaitingRecovery = true
		a.nextRps = a.recoveryRps
	case adaptivePhaseSteady:
		if failed {
			a.recordFailure(observation.TotalRPS)
			a.phase = adaptivePhaseSearch
			a.pendingSettle = a.searchConverged()
			a.awaitingRecovery = true
			a.nextRps = a.recoveryRps
			return
		}
		a.recordSuccess(observation.TotalRPS)
		a.nextRps = a.bestSustainableRps()
	}
}

func (a *AdaptiveExponentialLoadCalculator) thresholdExceeded(observation LoadObservation) bool {
	if observation.TimeoutRatio() >= 0.5 {
		return true
	}
	return a.maxLatencyMillis > 0 && observation.P99LatencyMillis > a.maxLatencyMillis
}

func (a *AdaptiveExponentialLoadCalculator) nextRampRps(previous int) int {
	if previous >= a.maxRps {
		return a.maxRps
	}
	if previous <= 0 {
		if a.maxRps <= 0 {
			return 0
		}
		if a.minRps > 1 {
			return a.minRps
		}
		return 1
	}
	next := previous * 2
	if next < a.minRps {
		next = a.minRps
	}
	return a.clampRps(next)
}

func (a *AdaptiveExponentialLoadCalculator) recordSuccess(rps int) {
	rps = a.clampRps(rps)
	if !a.highestSuccessfulKnown || rps > a.highestSuccessfulRps {
		a.highestSuccessfulRps = rps
		a.highestSuccessfulKnown = true
	}
}

func (a *AdaptiveExponentialLoadCalculator) recordFailure(rps int) {
	rps = a.clampRps(rps)
	if a.lowestUnsuccessfulRps < 0 || rps < a.lowestUnsuccessfulRps {
		a.lowestUnsuccessfulRps = rps
	}
}

func (a *AdaptiveExponentialLoadCalculator) bestSustainableRps() int {
	if a.highestSuccessfulKnown {
		return a.clampRps(a.highestSuccessfulRps)
	}
	return a.recoveryRps
}

func (a *AdaptiveExponentialLoadCalculator) searchConverged() bool {
	if a.lowestUnsuccessfulRps <= 0 {
		return false
	}
	low := a.bestSustainableRps()
	high := a.lowestUnsuccessfulRps
	if high <= low {
		return true
	}
	if high-low <= a.minBinaryGranularity {
		return true
	}
	_, ok := a.nextBinaryProbeRps()
	return !ok
}

func (a *AdaptiveExponentialLoadCalculator) nextBinaryProbeRps() (int, bool) {
	if a.lowestUnsuccessfulRps <= 0 {
		return 0, false
	}
	low := a.bestSustainableRps()
	high := a.lowestUnsuccessfulRps
	if high <= low || high-low <= a.minBinaryGranularity {
		return 0, false
	}

	midpoint := (low + high) / 2
	candidate := roundDownToMultiple(midpoint, a.minBinaryGranularity)
	if candidate <= low {
		candidate = roundUpToMultiple(low+1, a.minBinaryGranularity)
	}
	if candidate >= high {
		candidate = roundDownToMultiple(high-1, a.minBinaryGranularity)
	}
	candidate = a.clampRps(candidate)
	if candidate <= low || candidate >= high {
		return 0, false
	}
	return candidate, true
}

func (a *AdaptiveExponentialLoadCalculator) clampRps(rps int) int {
	if rps < 0 {
		rps = 0
	}
	if rps > a.maxRps {
		rps = a.maxRps
	}
	return rps
}

func roundDownToMultiple(value int, granularity int) int {
	if granularity <= 0 {
		return value
	}
	return int(math.Floor(float64(value)/float64(granularity))) * granularity
}

func roundUpToMultiple(value int, granularity int) int {
	if granularity <= 0 {
		return value
	}
	return int(math.Ceil(float64(value)/float64(granularity))) * granularity
}
