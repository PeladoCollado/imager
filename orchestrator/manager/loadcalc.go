package manager

type LoadCalculator interface {
	Next() int
}

func NewStepFunctionLoadCalculator(minRps int, maxRps int, stepSize int) LoadCalculator{
	return &StepFunctionLoadCalculator{minRps: minRps, maxRps: maxRps, stepSize: stepSize, currRps: minRps}
}

func NewExponentialLoadCalculator(minRps int, maxRps int) LoadCalculator {
	return &ExponentialFunctionLoadCalculator{minRps: minRps, maxRps: maxRps, factor: 2, currRps: minRps}
}

func NewLogarithmicLoadCalculator(minRps int, maxRps int) LoadCalculator {
	return &ExponentialFunctionLoadCalculator{minRps: minRps, maxRps: maxRps, factor: 10, currRps: minRps}
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
