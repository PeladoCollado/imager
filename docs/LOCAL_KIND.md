# Local Validation with kind or k3s

This project ships Kubernetes manifests for a complete local load-test stack:
- `imager-orchestrator` (single instance)
- `imager-executor` (scalable worker pool)
- `imager-test-service` (sample target service)

## Prerequisites

- Docker
- kind and kubectl (for kind workflow)
- A running k3s cluster (for k3s workflow)
- metrics-server installed in the cluster (required for pod/service target mode CPU/memory metrics)

## Build Images

```bash
docker build -t imager/orchestrator:local -f Dockerfile.orchestrator .
docker build -t imager/executor:local -f Dockerfile.executor .
```

## kind Workflow

1. Create cluster:
```bash
kind create cluster --name imager
```

2. Load images:
```bash
kind load docker-image imager/orchestrator:local --name imager
kind load docker-image imager/executor:local --name imager
```

3. Install metrics-server (if not already present):
```bash
kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml
```

4. Deploy stack (pod mode by default):
```bash
kubectl apply -k deploy/k8s
```

5. Check rollout:
```bash
kubectl -n imager get pods
kubectl -n imager get svc
```

6. Observe metrics endpoints:
```bash
kubectl -n imager port-forward svc/imager-orchestrator 8099:8099
# orchestrator metrics: http://localhost:8099/metrics

kubectl -n imager port-forward svc/imager-executor-metrics 9100:9100
# executor metrics: http://localhost:9100/metrics
```

## Switch to Service Mode

The default deployment targets one random pod from the target deployment.
In service mode, workers target the service endpoint (so Kubernetes load-balances across pods),
while the orchestrator still collects/report metrics across all backing pods:

```bash
kubectl apply -f deploy/k8s/orchestrator-service-mode.yaml
kubectl -n imager rollout status deploy/imager-orchestrator
```

## Switch to Arbitrary URL Mode

To target an external or non-cluster URL:

```bash
kubectl apply -f deploy/k8s/orchestrator-url-mode.yaml
kubectl -n imager rollout status deploy/imager-orchestrator
```

In URL mode:
- orchestrator does not collect pod CPU/memory metrics
- executor metrics still report request latency and errors (`imager_duration`, `imager_successDuration`, `imager_success`, `imager_failed`)

## Configure Requests

Update the request source by editing:
- `deploy/k8s/configmap-requests.yaml` for cluster deployment
- `deploy/examples/requests.json` as a local example source file

After ConfigMap updates:

```bash
kubectl -n imager apply -f deploy/k8s/configmap-requests.yaml
kubectl -n imager rollout restart deploy/imager-orchestrator
```

## k3s Workflow

1. Build and publish images to a registry reachable by your k3s cluster, then update image references in:
- `deploy/k8s/orchestrator-pod-mode.yaml`
- `deploy/k8s/orchestrator-service-mode.yaml`
- `deploy/k8s/orchestrator-url-mode.yaml` (if using URL mode)
- `deploy/k8s/executor.yaml`

2. Apply manifests:
```bash
kubectl apply -k deploy/k8s
```

3. Verify:
```bash
kubectl -n imager get pods,svc
kubectl -n imager logs deploy/imager-orchestrator
```

## Cleanup

```bash
kubectl delete -k deploy/k8s
kind delete cluster --name imager
```
