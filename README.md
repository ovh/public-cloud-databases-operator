# public-cloud-databases-operator

This operator allow you to automaticaly authorize your Kubernetes cluster IP on your OVHcloud cloud databases service.

## Requirement

### Public or private network

This operator is compatible with public and private managed database.

#### Managed database in private mode

If the managed database is deployed in `private` mode, `private` IP addresses of the Kubernetes nodes will be trusted.
The Kubernetes cluster must be deployed in the same network used by managed database.

#### Managed database in public mode and Kubernetes cluster `without` GW

If the managed database is deployed in `public` mode, `public` IP addresses of the Kubernetes nodes will be trusted.

#### Managed database in public mode and Kubernetes cluster `with` GW

The OVHcloud managed Kubernetes cluster can be configured to use a GW to reach internet.
If the managed database is deployed in `public` mode and a GW was configured on the managed Kubernetes, the `public` IP address of the GW will be trusted.

To determine the public IP address of the GW used by the Kubernetes cluster, the operator will request `https://ifconfig.io` and use the returned IP.

In this case, the operator must be deployed in the kubernetes cluster consuming targeted managed database.
If deployed outside the Kubernetes cluster, the returned IP address will be the public IP of the default GW of the machine running the operator. That can be different than the default GW used by Kubernetes nodes.

## Ovh Credentials

The operator needs a secret that contains the credentials to call Ovhcloud api. Go to <https://api.ovh.com/createToken/> to generate the credentials namely:

- application key
- application secret
- consumer key

Define the credentials ACL in order to be able to make these requests:

- GET /cloud/project/:projectID/database/service
- GET /cloud/project/:projectID/database/service/:serviceId
- PUT /cloud/project/:projectID/database/:engine/:serviceId

## Values

Create a values.yaml to be injected in the helm chart
that will be created afterwards. Region is either: ovh-eu, ovh-ca or ovh-us.
You can find the file in /examples.

```yaml
ovhCredentials:
  applicationKey: XXXX
  applicationSecret: XXXX
  consumerKey: XXXX
  region: XXXX

namespace: XXXX #Your Kubernetes namespace
```

## Installation

Use the kubernetes package manager [helm](https://helm.sh) and the values file you created to install the operator.

```bash
helm install -f values.yaml public-cloud-databases-operator oci://registry-1.docker.io/ovhcom/public-cloud-databases-operator
```

That will create the operator, crd and secrets.

 ```bash
kubectl get deploy
NAME                                       READY   UP-TO-DATE   AVAILABLE   AGE
operator-public-cloud-databases-operator   1/1     1            1           11h

kubectl get crd databases.cloud.ovh.net
NAME                      CREATED AT
databases.cloud.ovh.net   2023-06-01T14:20:09Z

kubectl get secret ovh-credentials
NAME              TYPE     DATA   AGE
ovh-credentials   Opaque   4      12m
```

## Create Custom Resource

Create a custom resource object using this example file.
You can find the file in /examples.

```yaml
apiVersion: cloud.ovh.net/v1alpha1
kind: Database
metadata:
  name: XXXX
  namespace: XXXX
spec:
  projectId: XXXX
  serviceId: XXX
  labelSelector:
    matchLabels:
      LABELNAME: LABELVALUE
```

The field serviceId is optional. If not set, the operator will be run against all the services of your project.

```bash
kubectl apply -f cr.yaml
```

## Nodes Labels

You can use kubernetes labeling in order to select specific nodes that you want the operator to be run against.
The created CR and the node must have the same label and value.

```bash
kubectl label nodes NODENAME1 NODENAME2 ... LABELNAME=LABELVALUE
```

## Additional IP addresses

Some hosts have to reach the managed database without being a node of the Kubernetes cluster,
for instance a bastion, a CI runner or a VPN endpoint. Those IP addresses cannot be discovered
from the Kubernetes API, so they can be declared on the CR with the
`databases.cloud.ovh.net/additional-ips` annotation.

```yaml
apiVersion: cloud.ovh.net/v1alpha1
kind: Database
metadata:
  name: XXXX
  namespace: XXXX
  annotations:
    databases.cloud.ovh.net/additional-ips: "203.0.113.5, 198.51.100.0/24"
spec:
  projectId: XXXX
```

They are authorized in addition to the IP addresses of the selected nodes. Entries are
separated by commas or whitespace, which also allows the multi line form:

```yaml
  annotations:
    databases.cloud.ovh.net/additional-ips: |
      203.0.113.5
      198.51.100.0/24
```

An entry is either a bare IP address, authorized as a single host (`/32` for IPv4, `/128`
for IPv6), or a CIDR block, authorized as a whole. A block is normalized to its network
address, so `192.0.2.10/24` is authorized as `192.0.2.0/24`.

These IP addresses are owned by the operator exactly like the node ones: they are removed
from the service when the annotation entry is removed or when the CR is deleted. An entry
that cannot be parsed is reported in the operator logs and ignored, the other entries and
the other CRs are still processed.

## Related links

- Contribute: <https://github.com/ovh/public-cloud-databases-operator/blob/master/CONTRIBUTING.md>
- Report bugs: <https://github.com/ovh/public-cloud-databases-operator/issues>

## License

Copyright 2021 OVH SAS

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
