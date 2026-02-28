package manager

import (
	"github.com/PeladoCollado/imager/orchestrator/logger"
	"github.com/PeladoCollado/imager/types"
	"go.uber.org/zap"
	"sync"
	"time"
)

// HeartbeatFrequencySeconds dictates the frequency of expected heartbeats from executors
const HeartbeatFrequencySeconds = 5

// MaxMissedHeartbeats dictates the maximum number of heartbeats missed before we declare the executor dead
const MaxMissedHeartbeats = 3
const heartbeatFailureDuration = HeartbeatFrequencySeconds * MaxMissedHeartbeats * time.Second

var executorMap = make(map[string]*Executor)
var lock sync.Mutex

// Executor represents a running client process that hosts worker threads. Each Executor can support up to #Workers
// simultaneously running Jobs.
type Executor struct {
	Id            string
	HeartbeatTime time.Time
	Workers       int
	WorkChan      chan []types.Job
}

// EligibleExecutors returns an array of Executors that are currently alive and ready to receive work
func EligibleExecutors() []*Executor {
	execs := make([]*Executor, 0, len(executorMap))
	now := time.Now()
	lock.Lock()
	defer lock.Unlock()
	for id := range executorMap {
		if now.After(executorMap[id].HeartbeatTime.Add(heartbeatFailureDuration)) {
			logger.Logger.Warn("Executor failed to heartbeat in time- deleting from registry",
				zap.String("executorId", id))
			delete(executorMap, id)
		} else {
			execs = append(execs, executorMap[id])
		}
	}

	return execs
}

// GetExecutor returns an Executor by its id, if it exists. nil is returned otherwise.
func GetExecutor(id string) *Executor {
	lock.Lock()
	defer lock.Unlock()
	return executorMap[id]
}

// CountExecutors returns the number of currently tracked executors.
func CountExecutors() int {
	lock.Lock()
	defer lock.Unlock()
	return len(executorMap)
}

// AddExecutor adds an Executor to the list of Executors to track.
func AddExecutor(id string, workerCount int) {
	lock.Lock()
	defer lock.Unlock()
	if _, ok := executorMap[id]; !ok {
		executorMap[id] = &Executor{Id: id,
			HeartbeatTime: time.Now(),
			Workers:       workerCount,
			WorkChan:      make(chan []types.Job)}
	}
	logger.Logger.Info("Added executor", executorMap[id])
}

// RecordHeartbeat records a heartbeat for an executor by its id.
func RecordHeartbeat(id string) {
	lock.Lock()
	defer lock.Unlock()
	if e, ok := executorMap[id]; ok {
		e.HeartbeatTime = time.Now()
	}
}

// ResetExecutors clears the in-memory executor registry.
func ResetExecutors() {
	lock.Lock()
	defer lock.Unlock()
	executorMap = make(map[string]*Executor)
}
