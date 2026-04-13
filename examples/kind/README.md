# Testing neo4g-operator on Kind

## Prerequisites

- [Docker](https://docs.docker.com/get-docker/)
- [Kind](https://kind.sigs.k8s.io/docs/user/quick-start/#installation)
- [kubectl](https://kubernetes.io/docs/tasks/tools/)
- Go 1.24+ (to build the operator)

## Quick Start

### 1. Create the Kind cluster

```bash
kind create cluster --config examples/kind/kind-config.yaml
```

This creates a 3-node cluster (1 control-plane + 2 workers) with port 7474 forwarded to localhost.

### 2. Build and load the operator image

```bash
make docker-build IMG=neo4g-operator:dev
kind load docker-image neo4g-operator:dev --name neo4g-dev
```

### 3. Install CRDs and deploy the operator

```bash
make install
make deploy IMG=neo4g-operator:dev
```

Verify the operator is running:

```bash
kubectl get pods -n neo4g-operator-system
```

### 4. Deploy a Neo4gCluster

**Single-node** (no gateway, minimal resources):

```bash
kubectl apply -f examples/kind/neo4gcluster-single.yaml
```

**HA cluster** (3 replicas + gateway):

```bash
kubectl apply -f examples/kind/neo4gcluster-ha.yaml
```

### 5. Check status

```bash
kubectl get neo4gclusters
kubectl describe neo4gcluster dev-graph
```

Watch pods come up:

```bash
kubectl get pods -w
```

### 6. Access Neo4g

The client service is available at `dev-graph.default.svc:7474` inside the cluster.

Port-forward to localhost:

```bash
kubectl port-forward svc/dev-graph 7474:7474
```

## Operator Logs

```bash
kubectl logs -n neo4g-operator-system deployment/neo4g-operator-controller-manager -c manager -f
```

## Cleanup

Delete the Neo4gCluster:

```bash
kubectl delete neo4gcluster dev-graph
```

Tear down the operator:

```bash
make undeploy
```

Delete the Kind cluster:

```bash
kind delete cluster --name neo4g-dev
```

## Troubleshooting

| Symptom | Fix |
|---------|-----|
| Pods stuck in `Pending` | Check PVC binding: `kubectl get pvc`. Kind uses `standard` StorageClass by default — no `storageClassName` needed. |
| StatefulSet not creating pods | Check operator logs for RBAC or image pull errors. |
| Gateway not deployed | Gateway only deploys when `spec.replicas > 1`. Use the HA manifest. |
| Image pull errors | Ensure you ran `kind load docker-image` for both the operator and neo4g images. |

### Loading the Neo4g image into Kind

If the neo4g image isn't on a public registry accessible from Kind nodes:

```bash
docker pull ghcr.io/neo4g/neo4g:latest
kind load docker-image ghcr.io/neo4g/neo4g:latest --name neo4g-dev
```
