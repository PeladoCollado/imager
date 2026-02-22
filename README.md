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

## 2. Run Imager On Its Own (Built-In Capabilities)

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

More local-cluster notes are in `docs/LOCAL_KIND.md`.

## 3. Customize Request Source And Load Calculator

### Config-only customization

1. File request source (no code changes):
- edit request specs in `deploy/k8s/configmap-requests.yaml` (`requests.json`)
- apply and restart orchestrator:
```bash
kubectl -n imager apply -f deploy/k8s/configmap-requests.yaml
kubectl -n imager rollout restart deploy/imager-orchestrator
```

2. Random-sum request source:
- in orchestrator args, set:
```yaml
- -request-source-type=random-sum
- -random-sum-path=/sum
- -random-sum-min=1
- -random-sum-max=1000
```
- remove `-request-source-file=...` and the `/config` ConfigMap volume mount from the orchestrator manifest

3. Built-in load calculators:
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

### Code-level customization

1. Custom request source:
- implement `types.RequestSource` in `types/types.go` (`Next()` and `Reset()`)
- add constructor in `orchestrator/requests/`
- wire it in `newRequestSource()` in `orchestrator/orchestrator.go`
- add flags/validation in `parseConfig()` and `validateConfig()`

2. Custom load calculator:
- implement `manager.LoadCalculator` in `orchestrator/manager/loadcalc.go` (`Next()`)
- wire it in `newLoadCalculator()` in `orchestrator/orchestrator.go`
- add/update unit tests in `orchestrator/manager/loadcalc_test.go`

3. Rebuild and redeploy:
```bash
go test ./...
docker build -t imager/orchestrator:local -f Dockerfile.orchestrator .
kind load docker-image imager/orchestrator:local --name kind
kubectl -n imager rollout restart deploy/imager-orchestrator
kubectl -n imager rollout status deploy/imager-orchestrator --timeout=180s
```
