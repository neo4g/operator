# neo4g-operator

> [!WARNING]
> This is a hobby/learning project and is **not ready for production use**. Expect breaking changes, missing features, and no stability guarantees.

Kubernetes operator for [Neo4g](https://github.com/neo4g/neo4g) — manages Neo4g graph database clusters on Kubernetes.

## Getting Started

### Prerequisites
- go version v1.24.6+
- docker version 17.03+
- kubectl version v1.11.3+
- Access to a Kubernetes v1.11.3+ cluster

### Install with Helm

```sh
helm install neo4g-operator oci://ghcr.io/neo4g/charts/neo4g-operator --version <version>
```

### Install with YAML

```sh
kubectl apply -f https://raw.githubusercontent.com/neo4g/operator/v<version>/dist/install.yaml
```

### Install from Source

**Build and push the image:**

```sh
make docker-build docker-push IMG=ghcr.io/neo4g/operator:tag
```

**Install CRDs and deploy the controller:**

```sh
make install
make deploy IMG=ghcr.io/neo4g/operator:tag
```

### Create a Neo4g Cluster

```sh
kubectl apply -k config/samples/
```

### Uninstall

```sh
kubectl delete -k config/samples/
make undeploy
make uninstall
```

## Development

```sh
make manifests generate  # Regenerate CRDs/RBAC/DeepCopy
make test                # Run unit tests
make lint                # Run linter
make run                 # Run controller locally
make test-e2e            # Run e2e tests (requires Kind)
```

Run `make help` for all available targets.

## Contributing

See the [Neo4g application repo](https://github.com/neo4g/neo4g) for the main project.

## License

Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
