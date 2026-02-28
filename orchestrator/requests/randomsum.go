package requests

import (
	"fmt"
	"github.com/PeladoCollado/imager/types"
	"math/rand"
	"net/http"
	"time"
)

type RandomSumSource struct {
	path string
	min  int
	max  int
	rng  *rand.Rand
}

func NewRandomSumSource(path string, min int, max int) (types.RequestSource, error) {
	if max < min {
		return nil, fmt.Errorf("max must be >= min")
	}
	if path == "" {
		path = "/sum"
	}
	return &RandomSumSource{
		path: path,
		min:  min,
		max:  max,
		rng:  rand.New(rand.NewSource(time.Now().UnixNano())),
	}, nil
}

func (r *RandomSumSource) Next() (types.RequestSpec, error) {
	a := r.nextValue()
	b := r.nextValue()
	return types.RequestSpec{
		Method:      http.MethodGet,
		Path:        r.path,
		QueryString: fmt.Sprintf("a=%d&b=%d", a, b),
	}, nil
}

func (r *RandomSumSource) Reset() error {
	return nil
}

func (r *RandomSumSource) nextValue() int {
	if r.max == r.min {
		return r.min
	}
	return r.min + r.rng.Intn(r.max-r.min+1)
}
