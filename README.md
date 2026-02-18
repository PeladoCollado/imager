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

## Local Deployment

Kubernetes manifests and local cluster instructions are under:
- `deploy/k8s/`
- `docs/LOCAL_KIND.md`
