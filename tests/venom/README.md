# Venom integration tests

Integration tests for the operator and its Helm chart, written for
[Venom](https://github.com/ovh/venom) (v1.2+).

## Suites

| Suite | Needs | What it covers |
| ----- | ----- | -------------- |
| `01-helm-chart.yml` | `helm` only | Chart lints, templates from the tree **and from the packaged .tgz** (regression for [#31](https://github.com/ovh/public-cloud-databases-operator/issues/31)), credentials wiring, `existingSecret` behavior |
| `02-operator-e2e.yml` | kubectl context, OVH credentials, a dedicated test database service | Chart install, node/gateway IP authorization on the service, preservation of foreign IP restrictions (multicluster guarantee), cleanup on CR deletion |

## Running

Chart suite (no cluster, no credentials):

```sh
make venom-test-chart
```

End-to-end suite:

1. Point `kubectl` at a disposable cluster.
2. `cp variables.yaml.example variables.yaml` and fill in the OVH
   credentials and the target `projectId`/`serviceId`/`engine`.
   **The suite rewrites and finally wipes the service's IP
   restrictions** — use a service dedicated to testing.
3. ```sh
   make venom-test-e2e
   ```

The e2e suite installs the chart into its own namespace (`pcdb-venom`)
and removes everything it created on success.
