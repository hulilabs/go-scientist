package scientist

import (
	"errors"
	"fmt"
	"time"
)

const (
	controlBehavior   = "control"
	candidateBehavior = "candidate"
)

func Bool(ok interface{}, err error) (bool, error) {
	if err != nil {
		return false, err
	}

	switch t := ok.(type) {
	case bool:
		return t, nil
	default:
		return false, fmt.Errorf("[scientist] bad result type: %v (%T)", ok, ok)
	}
}

type Observation struct {
	Experiment *Experiment
	Name       string
	Started    time.Time
	Runtime    time.Duration
	Value      interface{}
	Err        error
}

func (o *Observation) CleanedValue() (interface{}, error) {
	return o.Experiment.cleaner(o.Value)
}

type Result struct {
	Experiment   *Experiment
	Control      *Observation
	Observations []*Observation
	Candidates   []*Observation
	Ignored      []*Observation
	Mismatched   []*Observation
	Errors       []ResultError
}

func Run(e *Experiment, name string) Result {
	r := Result{Experiment: e}
	if err := e.beforeRun(); err != nil {
		r.Errors = append(r.Errors, e.resultErr("before_run", err))
	}

	numCandidates := len(e.behaviors) - 1
	r.Control = observe(e, name, e.behaviors[name])
	r.Candidates = make([]*Observation, numCandidates)
	r.Ignored = make([]*Observation, 0, numCandidates)
	r.Mismatched = make([]*Observation, 0, numCandidates)
	r.Observations = make([]*Observation, numCandidates+1)
	r.Observations[0] = r.Control

	i := 0
	for bname, b := range e.behaviors {
		if bname == name {
			continue
		}

		c := observe(e, bname, b)
		r.Candidates[i] = c
		i += 1
		r.Observations[i] = c

		mismatched, err := mismatching(e, r.Control, c)
		if err != nil {
			mismatched = true
			r.Errors = append(r.Errors, e.resultErr("compare", err))
		}

		if !mismatched {
			continue
		}

		ignored, err := ignoring(e, r.Control, c)
		if err != nil {
			ignored = false
			r.Errors = append(r.Errors, e.resultErr("ignore", err))
		}

		if ignored {
			r.Ignored = append(r.Ignored, c)
		} else {
			r.Mismatched = append(r.Mismatched, c)
		}
	}

	if err := e.publisher(r); err != nil {
		r.Errors = append(r.Errors, e.resultErr("publish", err))
	}

	if len(r.Errors) > 0 {
		e.errorReporter(r.Errors...)
	}

	return r
}

func mismatching(e *Experiment, control, candidate *Observation) (bool, error) {
	matching, err := e.comparator(control.Value, candidate.Value)
	return !matching, err
}

func ignoring(e *Experiment, control, candidate *Observation) (bool, error) {
	for _, i := range e.ignores {
		ok, err := i(control.Value, candidate.Value)
		if err != nil {
			return false, err
		}

		if ok {
			return true, nil
		}
	}

	return false, nil
}

func behaviorNotFound(e *Experiment, name string) error {
	return fmt.Errorf("Behavior %q not found for experiment %q", name, e.Name)
}

func observe(e *Experiment, name string, b behaviorFunc) *Observation {
	o := &Observation{
		Experiment: e,
		Name:       name,
		Started:    time.Now(),
	}

	if b == nil {
		b = e.behaviors[name]
	}

	if b == nil {
		o.Runtime = time.Since(o.Started)
		o.Err = behaviorNotFound(e, name)
	} else {
		v, err := runBehavior(e, name, b)
		o.Runtime = time.Since(o.Started)
		o.Value = v
		o.Err = err
	}

	return o
}

func runBehavior(e *Experiment, name string, b behaviorFunc) (value interface{}, err error) {
	defer func() {
		if er := recover(); er != nil {
			value = nil
			switch t := er.(type) {
			case string:
				err = errors.New(t)
			case error:
				err = t
			default:
				err = fmt.Errorf("%v", t)
			}
		}
	}()
	value, err = b()
	return
}

type ResultError struct {
	Operation  string
	Experiment string
	Err        error
}

func (e ResultError) Error() string {
	return e.Err.Error()
}
