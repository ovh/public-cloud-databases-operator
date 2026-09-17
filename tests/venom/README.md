# Venom integration tests

Integration tests for the operator and its Helm chart, written for
[Venom](https://github.com/ovh/venom) (v1.2+).

## Suites

| Suite | Needs | What it covers |
| ----- | ----- | -------------- |
| `01-helm-chart.yml` | `helm` only | Chart lints, templates from the tree **and from the packaged .tgz** (regression for [#31](https://github.com/ovh/public-cloud-databases-operator/issues/31)), credentials wiring, `existingSecret` behavior |
| `02-operator-e2e.yml` | `helm`, `kubectl`, `jq`, OVH credentials | **Self-provisioning end-to-end**: creates a managed Kubernetes cluster and a PostgreSQL service in the project, installs the chart, verifies ip opening (cluster IPs authorized on the service), `spec.additionalIps` (admission rejection of invalid entries, bare-IP/CIDR normalization, revocation on update, no takeover of a colliding foreign entry), preservation of foreign IP restrictions (multicluster guarantee) and cleanup on CR deletion, then deletes everything it created |

## Running

Chart suite (no cluster, no credentials):

```sh
make venom-test-chart
```

End-to-end suite:

1. `cp variables.yaml.example variables.yaml` and fill in an OVH API
   token allowed to create/manage/delete managed Kubernetes clusters
   **and** database services in the project, plus the `projectId`.
2. ```sh
   make venom-test-e2e
   ```

Mind the cost and duration: the suite provisions a real managed
Kubernetes cluster (1 node) and a PostgreSQL essential service, both
billed for the run's ~30-45 minutes, and deletes them in its final
`teardown` testcase. If the run aborts before teardown, delete the
resources named `pcdb-venom-<suffix>` from the project by hand (the
default suffix is `local`; CI uses the CDS run number).

## CI

`.cds/workflows/build.yml` runs `make test` + the chart suite on every
push (`Test` job), and the full e2e suite on tags (`E2E` job). The
`Release` job needs all of them, so a release cannot ship without the
e2e suite passing. Credentials come from the `pcdb-e2e` CDS variable
set (`ovh_application_key`, `ovh_application_secret`,
`ovh_consumer_key`, `project_id`).

The `spec.additionalIps` testcases need an operator image that carries
the feature: leave `imageRepository`/`imageTag` empty to use the chart
default (fine once a release ships the feature), or point them at a
freshly built image — CI sets them to the image built from the same
commit via the `docker_registry` variable-set item.
