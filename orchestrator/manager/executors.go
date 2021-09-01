package manager

import (
	"github.com/PeladoCollado/imager/orchestrator/logger"
	"github.com/PeladoCollado/imager/types"
	"go.uber.org/zap"
	"sync"
	"time"
)

const HeartbeatFrequencySeconds = 5
const MaxMissedHeartbeats = 3
const heartbeatFailureDuration = HeartbeatFrequencySeconds * MaxMissedHeartbeats * time.Second

var executorMap = make(map[string]*Executor)
var lock sync.Mutex

type Executor struct {
	Id            string
	HeartbeatTime time.Time
	Workers       int
	WorkChan      chan []types.Job
}

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

func GetExecutor(id string) *Executor {
	lock.Lock()
	defer lock.Unlock()
	return executorMap[id]
}

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

func RecordHeartbeat(id string) {
	if e, ok := executorMap[id]; ok {
		e.HeartbeatTime = time.Now()
	}
}
