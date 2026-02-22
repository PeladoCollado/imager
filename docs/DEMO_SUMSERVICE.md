# Sum Service Demo on kind

This demo deploys:
- `sumservice` in namespace `imagerdemo`
- `imager-orchestrator` + `imager-executor` in namespace `imager`

Imager is configured to:
- target a random pod from deployment `sumservice` in namespace `imagerdemo`
- generate random request pairs (`a`, `b`) at `/sum`
- use step load: 10 rps, +10 rps each schedule tick, capped at 500 rps

## Build images

```bash
docker build -t imager/orchestrator:local -f Dockerfile.orchestrator .
docker build -t imager/executor:local -f Dockerfile.executor .
docker build -t imager/sumservice:local -f Dockerfile.sumservice .
```

## Create/reset kind cluster

```bash
kind delete cluster --name kind || true
kind create cluster --name kind
```

## Install metrics-server

```bash
kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml
kubectl -n kube-system rollout status deploy/metrics-server --timeout=180s
```

## Load images into kind

```bash
kind load docker-image imager/orchestrator:local --name kind
kind load docker-image imager/executor:local --name kind
kind load docker-image imager/sumservice:local --name kind
```

## Deploy demo stack

```bash
kubectl apply -k deploy/demo
kubectl -n imagerdemo rollout status deploy/sumservice --timeout=180s
kubectl -n imager rollout status deploy/imager-orchestrator --timeout=180s
kubectl -n imager rollout status deploy/imager-executor --timeout=180s
```

## Validate service behavior

```bash
kubectl -n imagerdemo port-forward svc/sumservice 8088:8080
curl 'http://127.0.0.1:8088/sum?a=7&b=9'
# expected: {"a":7,"b":9,"sum":16}
```

## Validate imager is running load

```bash
kubectl -n imager port-forward svc/imager-orchestrator 8099:8099
curl -s http://127.0.0.1:8099/metrics | rg 'imager_orchestrator_(registered_executors|jobs_dispatched_total|job_requests_total|target_pod_)'
```

Expected checks:
- `imager_orchestrator_registered_executors` equals executor replica count (5)
- `imager_orchestrator_jobs_dispatched_total` increases over time
- `imager_orchestrator_job_requests_total` increases over time
- `imager_orchestrator_target_pod_cpu_millicores{namespace="imagerdemo",pod="..."}` is present
- `imager_orchestrator_target_pod_memory_bytes{namespace="imagerdemo",pod="..."}` is present

For one executor pod metrics:

```bash
POD=$(kubectl -n imager get pod -l app=imager-executor -o jsonpath='{.items[0].metadata.name}')
kubectl -n imager port-forward pod/$POD 9100:9100
curl -s http://127.0.0.1:9100/metrics | rg 'imager_(executor_jobs_picked_up_total|executor_job_requests_total|success|failed)'
```

Expected checks:
- `imager_executor_jobs_picked_up_total` > 0
- `imager_executor_job_requests_total` > 0
- `imager_success` increases
- `imager_failed` stays near 0
