# imager
imager load test tool

## Go Version
This repository is configured for modern Go tooling with:
- `go 1.26.0`
- `toolchain go1.26.0`

If your local machine is on an older Go release, install Go 1.26.x and run:

```bash
go mod tidy
```

## Overview

Imager is a Kubernetes-native distributed load-testing system with two processes:
- `orchestrator` (single instance): discovers target pods/services, schedules load, dispatches jobs, and publishes orchestrator metrics.
- `executor` (N replicas): registers with the orchestrator, picks up jobs, executes HTTP request batches, and publishes executor metrics.

## 1. Run The Demo (sumservice on kind)

This demo deploys:
- `sumservice` in namespace `imagerdemo`
- `imager-orchestrator` and `imager-executor` in namespace `imager`

1. Build images:
```bash
docker build -t imager/orchestrator:local -f Dockerfile.orchestrator .
docker build -t imager/executor:local -f Dockerfile.executor .
docker build -t imager/sumservice:local -f Dockerfile.sumservice .
```

2. Create a local cluster and install metrics-server:
```bash
kind delete cluster --name kind || true
kind create cluster --name kind
kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml
kubectl -n kube-system rollout status deploy/metrics-server --timeout=180s
```

3. Load images and deploy the demo manifests:
```bash
kind load docker-image imager/orchestrator:local --name kind
kind load docker-image imager/executor:local --name kind
kind load docker-image imager/sumservice:local --name kind
kubectl apply -k deploy/demo
kubectl -n imagerdemo rollout status deploy/sumservice --timeout=180s
kubectl -n imager rollout status deploy/imager-orchestrator --timeout=180s
kubectl -n imager rollout status deploy/imager-executor --timeout=180s
```

4. Verify endpoint and metrics:
```bash
kubectl get --raw '/api/v1/namespaces/imagerdemo/services/http:sumservice:8080/proxy/sum?a=7&b=9'
kubectl get --raw '/api/v1/namespaces/imager/services/http:imager-orchestrator:8099/proxy/metrics' | rg 'imager_orchestrator_(registered_executors|jobs_dispatched_total|job_requests_total|target_pod_)'
kubectl get --raw '/api/v1/namespaces/imager/services/http:imager-executor-metrics:9100/proxy/metrics' | rg 'imager_(executor_jobs_picked_up_total|executor_job_requests_total|success|failed)'
```

For a complete demo runbook, see `docs/DEMO_SUMSERVICE.md`.

## 2. Run Imager On Its Own (Built-In + Config-Only Customization)

If you want to run the load generator without the sumservice demo, use the built-in stack in `deploy/k8s`:
- built-in target deployment/service: `imager-test-service`
- built-in request source: `deploy/k8s/configmap-requests.yaml`
- built-in load profile: step (`min-rps=5`, `max-rps=50`, `step-rps=5`)

1. Ensure a local cluster exists and metrics-server is available:
```bash
kind get clusters
kubectl -n kube-system get deploy metrics-server
```

2. Build and load orchestrator/executor images:
```bash
docker build -t imager/orchestrator:local -f Dockerfile.orchestrator .
docker build -t imager/executor:local -f Dockerfile.executor .
kind load docker-image imager/orchestrator:local --name kind
kind load docker-image imager/executor:local --name kind
```

3. Deploy built-in manifests:
```bash
kubectl apply -k deploy/k8s
kubectl -n imager rollout status deploy/imager-orchestrator --timeout=180s
kubectl -n imager rollout status deploy/imager-executor --timeout=180s
kubectl -n imager rollout status deploy/imager-test-service --timeout=180s
```

4. Verify it is actively running:
```bash
kubectl -n imager get pods,svc
kubectl get --raw '/api/v1/namespaces/imager/services/http:imager-orchestrator:8099/proxy/metrics' | rg 'imager_orchestrator_(registered_executors|jobs_dispatched_total|job_requests_total)'
```

5. Optional: switch orchestrator to service target mode:
```bash
kubectl apply -f deploy/k8s/orchestrator-service-mode.yaml
kubectl -n imager rollout status deploy/imager-orchestrator --timeout=180s
```

### Config-only customization (no code changes)

#### StreamReader file source (JSON records, one per line)

The built-in `file` request source uses `StreamReader`, which reads newline-delimited JSON records.
Each line is one `RequestSpec` object. Supported fields:
- `method` (required)
- `path` (required)
- `queryString` (optional)
- `headers` (optional map of string to array of strings)
- `body` (optional)

Example file:

```json
{"method":"GET","path":"/"}
{"method":"GET","path":"/status"}
{"method":"GET","path":"/anything","headers":{"X-Imager-Run":["true"]}}
{"method":"POST","path":"/submit","headers":{"Content-Type":["application/json"]},"body":"{\"run\":\"demo\"}"}
```

You can start from `deploy/examples/requests.json`.

To configure deployment to use this file:

1. Put the records into the cluster ConfigMap used by the orchestrator:
```bash
kubectl -n imager create configmap imager-request-source \
  --from-file=requests.json=./deploy/examples/requests.json \
  --dry-run=client -o yaml | kubectl apply -f -
```

2. Ensure orchestrator deployment points to file mode and the mounted path:
- `-request-source-type=file`
- `-request-source-file=/config/requests.json`
- volume mount at `/config` from ConfigMap `imager-request-source`

The default manifests in `deploy/k8s/orchestrator-pod-mode.yaml` and
`deploy/k8s/orchestrator-service-mode.yaml` already use this path. If you changed them,
re-apply your chosen manifest before restarting:
```bash
kubectl apply -f deploy/k8s/orchestrator-pod-mode.yaml
# or
kubectl apply -f deploy/k8s/orchestrator-service-mode.yaml
```

3. Restart orchestrator:
```bash
kubectl -n imager rollout restart deploy/imager-orchestrator
kubectl -n imager rollout status deploy/imager-orchestrator --timeout=180s
```

If you use a different filename or mount path, update `-request-source-file` accordingly.

#### Random-sum request source

In orchestrator args, set:
```yaml
- -request-source-type=random-sum
- -random-sum-path=/sum
- -random-sum-min=1
- -random-sum-max=1000
```

If you switch fully to random-sum, remove `-request-source-file=...` and the `/config` ConfigMap volume mount.

#### Built-in load calculators

- `-load-calculator=step` with `-min-rps`, `-max-rps`, `-step-rps`
- `-load-calculator=exponential` with `-min-rps`, `-max-rps`
- `-load-calculator=logarithmic` with `-min-rps`, `-max-rps`

Example step profile from 10 to 500 rps:
```yaml
- -load-calculator=step
- -min-rps=10
- -max-rps=500
- -step-rps=10
```

More local-cluster notes are in `docs/LOCAL_KIND.md`.

## 3. Code-level customization

Imager exposes an importable orchestrator runtime in `github.com/PeladoCollado/imager/orchestrator/app`.
You can start the orchestrator from another project and inject custom factories without patching this repo.

1. Implement custom components in your project:
- a request source implementing `types.RequestSource`
- a load calculator implementing `manager.LoadCalculator`

Example: database-backed request source (reads one record per `Next()` call and loops on EOF):

```go
package requests

import (
	"database/sql"
	"errors"
	"sync"

	"github.com/PeladoCollado/imager/types"
)

type DBRequestSource struct {
	db     *sql.DB
	mu     sync.Mutex
	offset int
}

func NewDBRequestSource(db *sql.DB) *DBRequestSource {
	return &DBRequestSource{db: db}
}

func (s *DBRequestSource) Next() (types.RequestSpec, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	req, err := s.queryByOffset(s.offset)
	if errors.Is(err, sql.ErrNoRows) {
		s.offset = 0
		req, err = s.queryByOffset(s.offset)
	}
	if err != nil {
		return types.RequestSpec{}, err
	}
	s.offset++
	return req, nil
}

func (s *DBRequestSource) queryByOffset(offset int) (types.RequestSpec, error) {
	row := s.db.QueryRow(`
		SELECT method, path, query_string, body
		FROM request_specs
		ORDER BY id LIMIT 1 OFFSET ?`, offset)

	var req types.RequestSpec
	var queryString sql.NullString
	var body sql.NullString
	err := row.Scan(&req.Method, &req.Path, &queryString, &body)
	if err != nil {
		return types.RequestSpec{}, err
	}

	if queryString.Valid {
		req.QueryString = queryString.String
	}
	if body.Valid {
		req.Body = body.String
	}
	return req, nil
}

func (s *DBRequestSource) Reset() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.offset = 0
	return nil
}
```

2. Implement a custom load calculator:

Example: random spike load calculator (baseline load with occasional large spikes):

```go
package manager

import (
	"math/rand"
	"time"
)

type RandomSpikeLoadCalculator struct {
	minRPS      int
	maxRPS      int
	baselineRPS int
	maxSpikeRPS int
	spikeChance int // 0-100
	rng         *rand.Rand
}

func NewRandomSpikeLoadCalculator(minRPS, maxRPS, baselineRPS, maxSpikeRPS, spikeChance int) LoadCalculator {
	return &RandomSpikeLoadCalculator{
		minRPS:      minRPS,
		maxRPS:      maxRPS,
		baselineRPS: baselineRPS,
		maxSpikeRPS: maxSpikeRPS,
		spikeChance: spikeChance,
		rng:         rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (c *RandomSpikeLoadCalculator) Next() int {
	rps := c.baselineRPS
	if c.rng.Intn(100) < c.spikeChance {
		rps += c.rng.Intn(c.maxSpikeRPS + 1)
	}
	if rps < c.minRPS {
		return c.minRPS
	}
	if rps > c.maxRPS {
		return c.maxRPS
	}
	return rps
}
```

3. Start the orchestrator with custom factories from an external project:

```go
package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"syscall"

	imagerapp "github.com/PeladoCollado/imager/orchestrator/app"
	"github.com/PeladoCollado/imager/orchestrator/manager"
	"github.com/PeladoCollado/imager/types"
)

func main() {
	cfg, err := imagerapp.ParseConfig(os.Args[1:])
	if err != nil {
		log.Fatalf("parse config: %v", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	opts := imagerapp.RunOptions{
		RequestSourceFactory: imagerapp.RequestSourceFactoryFunc(func(cfg imagerapp.Config) (types.RequestSource, error) {
			if cfg.RequestSourceType == "db" {
				// initialize DB connection/pool once in real code and reuse it.
				return NewDBRequestSource(openDB()), nil
			}
			return imagerapp.NewBuiltInRequestSource(cfg)
		}),
		LoadCalculatorFactory: imagerapp.LoadCalculatorFactoryFunc(func(cfg imagerapp.Config) (manager.LoadCalculator, error) {
			if cfg.LoadCalculator == "random-spike" {
				return NewRandomSpikeLoadCalculator(cfg.MinRPS, cfg.MaxRPS, cfg.MinRPS, 400, 15), nil
			}
			return imagerapp.NewBuiltInLoadCalculator(cfg)
		}),
	}

	if err := imagerapp.Run(ctx, cfg, opts); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatalf("run orchestrator: %v", err)
	}
}
```

With this setup:
- `-request-source-type=db` uses your DB-backed source
- `-load-calculator=random-spike` uses your spike calculator
- all other values continue using built-in behavior via `NewBuiltInRequestSource` and `NewBuiltInLoadCalculator`

4. Rebuild and redeploy:
```bash
go test ./...
docker build -t imager/orchestrator:local -f Dockerfile.orchestrator .
kind load docker-image imager/orchestrator:local --name kind
kubectl -n imager rollout restart deploy/imager-orchestrator
kubectl -n imager rollout status deploy/imager-orchestrator --timeout=180s
```
