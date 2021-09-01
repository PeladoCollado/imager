package api

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/PeladoCollado/imager/orchestrator/logger"
	"github.com/PeladoCollado/imager/orchestrator/manager"
	"github.com/PeladoCollado/imager/types"
	"net/http"
	"time"
)
var ctx context.Context

type HttpError struct {
	code int
	err error
}

func (h *HttpError) Error() string {
	return h.err.Error()
}

func Init(p int, c context.Context) {
	http.HandleFunc("/connect", Connect)
	http.HandleFunc("/heartbeat", Heartbeat)
	http.HandleFunc("/next", Next)
	http.ListenAndServe(fmt.Sprintf(":%d", p), nil)
	ctx = c
}

func Connect(w http.ResponseWriter, r *http.Request) {
	workerId, httpError := parseWorkerId(r)
	if httpError != nil {
		w.WriteHeader(httpError.code)
		fmt.Fprint(w, httpError.Error())
	}
	manager.AddExecutor(workerId.Id, workerId.Workers)
	w.WriteHeader(http.StatusCreated)
}

func Heartbeat(w http.ResponseWriter, r *http.Request) {
	executor, err := findExecutor(r)
	if err != nil {
		w.WriteHeader(err.code)
		fmt.Fprint(w, err.Error())
	}
	executor.HeartbeatTime = time.Now()
}

func Next(w http.ResponseWriter, r *http.Request) {
	executor, err := findExecutor(r)
	if err != nil {
		w.WriteHeader(err.code)
		fmt.Fprint(w, err.Error())
	}
	logger.Logger.Info("Fetching next job for executor", executor.Id)
	select {
	case jobs := <-executor.WorkChan:
		logger.Logger.Info("Found jobs for executor", executor.Id, jobs)
		encoder := json.NewEncoder(w)
		encoder.Encode(jobs)
	case <- ctx.Done():
		logger.Logger.Warn("Context canceled- abandoning request", ctx.Err())
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("Server shutting down"))
	}
}

func findExecutor(r *http.Request) (*manager.Executor, *HttpError){
	workerId, httpError := parseWorkerId(r)
	if httpError != nil {
		return nil, httpError
	}

	executor := manager.GetExecutor(workerId.Id)
	if executor == nil {
		logger.Logger.Error("Unable to find executor by id", workerId)
		return nil, &HttpError{code: http.StatusNotFound,
			err: fmt.Errorf("Unable to find executor by id %s", workerId.Id)}
	}
	return executor, nil
}

func parseWorkerId(r *http.Request) (*types.WorkerId,*HttpError) {
	decoder := json.NewDecoder(r.Body)
	workerId := &types.WorkerId{}
	err := decoder.Decode(workerId)
	if err != nil {
		logger.Logger.Error("Unable to parse request", err)
		return nil, &HttpError{code: http.StatusInternalServerError, err: err}
	}
	return workerId, nil
}