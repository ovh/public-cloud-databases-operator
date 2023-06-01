# public-cloud-databases-operator
 
This operator allow you to automaticaly authorize your Kubernetes cluster IP on your OVHcloud cloud databases service.

## Ovh Credentials
The operator needs a secret that contains the credentials to call Ovhcloud api. Go to https://api.ovh.com/createToken/ to generate the credentials namely:
- application key
- application secret
- consumer key

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
helm install -f values.yaml public-cloud-databases-operator oci://registry-1.docker.io/ovhcom/public-cloud-databases-operator --version 3
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

# Nodes Labels
You can use kubernetes labeling in order to select specific nodes that you want the operator to be run against. 
The created CR and the node must have the same label and value.

```bash
kubectl label nodes NODENAME1 NODENAME2 ... LABELNAME=LABELVALUE
```

# Related links
 
 * Contribute: https://github.com/ovh/public-cloud-databases-operator/blob/master/CONTRIBUTING.md
 * Report bugs: https://github.com/ovh/public-cloud-databases-operator/issues

# License
 
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