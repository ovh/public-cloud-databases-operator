# config

Generated manifests, not an install path. The operator is installed with
the Helm chart in [`deploy/public-cloud-databases-operator`](../deploy/public-cloud-databases-operator).

| file | produced by | consumed by |
| ---- | ----------- | ----------- |
| `crd/bases/cloud.ovh.net_databases.yaml` | `make manifests` | the envtest suite, and the chart's CRD template |
| `rbac/role.yaml` | `make manifests` | reference for the chart's `manager-role.yaml` |

Both are regenerated from the kubebuilder markers in `api/` and
`controllers/`; edit the markers, not these files.

The kustomize overlays that used to live here described a second, drifting
copy of the deployment: they carried no OVHcloud credentials, so the
operator they installed could not authenticate, and they pinned a
kube-rbac-proxy image from a registry that no longer serves it. They were
removed rather than repaired, to keep one supported way to install the
operator.
