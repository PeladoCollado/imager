package api

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/PeladoCollado/imager/orchestrator/logger"
	"github.com/PeladoCollado/imager/orchestrator/manager"
	"github.com/PeladoCollado/imager/types"
	"net/http"
)

type HttpError struct {
	code int
	err  error
}

func (h *HttpError) Error() string {
	return h.err.Error()
}

func NewHandler(ctx context.Context) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/connect", connectHandler)
	mux.HandleFunc("/heartbeat", heartbeatHandler)
	mux.HandleFunc("/next", nextHandler(ctx))
	return mux
}

func Init(p int, c context.Context) error {
	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", p),
		Handler: NewHandler(c),
	}
	return server.ListenAndServe()
}

func connectHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	workerId, httpError := parseWorkerId(r)
	if httpError != nil {
		w.WriteHeader(httpError.code)
		_, _ = fmt.Fprint(w, httpError.Error())
		return
	}
	manager.AddExecutor(workerId.Id, workerId.Workers)
	w.WriteHeader(http.StatusCreated)
}

func heartbeatHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	workerId, httpError := parseWorkerId(r)
	if httpError != nil {
		w.WriteHeader(httpError.code)
		_, _ = fmt.Fprint(w, httpError.Error())
		return
	}
	executor := manager.GetExecutor(workerId.Id)
	if executor == nil {
		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprintf(w, "Unable to find executor by id %s", workerId.Id)
		return
	}
	manager.RecordHeartbeat(workerId.Id)
	w.WriteHeader(http.StatusOK)
}

func nextHandler(ctx context.Context) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		executor, httpError := findExecutor(r)
		if httpError != nil {
			w.WriteHeader(httpError.code)
			_, _ = fmt.Fprint(w, httpError.Error())
			return
		}
		logger.Logger.Info("Fetching next job for executor", executor.Id)
		select {
		case jobs := <-executor.WorkChan:
			logger.Logger.Info("Found jobs for executor", executor.Id, jobs)
			w.Header().Set("Content-Type", "application/json")
			encoder := json.NewEncoder(w)
			if err := encoder.Encode(jobs); err != nil {
				logger.Logger.Error("Unable to encode jobs response", err)
			}
		case <-ctx.Done():
			logger.Logger.Warn("Context canceled- abandoning request", ctx.Err())
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("Server shutting down"))
		}
	}
}

func findExecutor(r *http.Request) (*manager.Executor, *HttpError) {
	workerId, httpError := parseWorkerId(r)
	if httpError != nil {
		return nil, httpError
	}

	executor := manager.GetExecutor(workerId.Id)
	if executor == nil {
		logger.Logger.Error("Unable to find executor by id", workerId)
		return nil, &HttpError{
			code: http.StatusNotFound,
			err:  fmt.Errorf("unable to find executor by id %s", workerId.Id),
		}
	}
	return executor, nil
}

func parseWorkerId(r *http.Request) (*types.WorkerId, *HttpError) {
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	workerId := &types.WorkerId{}
	err := decoder.Decode(workerId)
	if err != nil {
		logger.Logger.Error("Unable to parse request", err)
		return nil, &HttpError{code: http.StatusBadRequest, err: err}
	}
	if workerId.Id == "" {
		return nil, &HttpError{code: http.StatusBadRequest, err: fmt.Errorf("id is required")}
	}
	if workerId.Workers <= 0 {
		workerId.Workers = 1
	}
	return workerId, nil
}
