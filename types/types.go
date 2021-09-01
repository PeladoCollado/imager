package types

import (
	"net/http"
	"time"
)

type Job struct {
	Source     RequestSource `json:source`
	RatePerSec int           `json:ratePerSec`
	Duration   time.Duration `json:duration`
}

type RequestSource interface {
	Next() (*http.Request, error)
	Read(resp *http.Response) (int64, error)
}

type WorkerId struct {
	Id      string `json:id`
	Workers int    `json:workers`
}
