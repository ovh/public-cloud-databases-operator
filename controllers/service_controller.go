/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/ovh/go-ovh/ovh"
	"github.com/ovh/public-cloud-databases-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ServiceReconciler reconciles a service object
type ServiceReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	Log       logr.Logger
	OvhClient ovh.Client
}

//+kubebuilder:rbac:groups=service.clouddb.ovhcloud.net,resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=service.clouddb.ovhcloud.net,resources=services/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=service.clouddb.ovhcloud.net,resources=services/finalizers,verbs=update
//+kubebuilder:rbac:groups=service.clouddb.ovhcloud.net,resources=nodes,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the service object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.13.0/pkg/reconcile
func (r *ServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.Log.WithValues("req", req)
	logger.Info("reconcile")

	//v1alpha1.SchemeBuilder.
	serviceList := &v1alpha1.ServiceList{}
	err := r.List(ctx, serviceList)
	if err != nil {
		logger.Info("failed to list crd", "error", err)
		return ctrl.Result{}, err
	}

	for _, crd := range serviceList.Items {
		opts := []client.ListOption{
			client.InNamespace(req.NamespacedName.Namespace),
		}
		for k, v := range crd.Spec.LabelSelector.MatchLabels {
			opts = append(opts, client.MatchingFields{k: v})
		}

		nodes := &corev1.NodeList{}
		err = r.List(ctx, nodes, opts...)
		if err != nil {
			logger.Info("failed to list nodes", "error", err)
			return ctrl.Result{}, err
		}
		// check if there is a wildcard on service id, then process on all the services of the project
		if crd.Spec.ServiceId == "" {
			servicesIds, err := GetServicesForProjectId(ctx, r, crd.Spec.ProjectId)
			if err != nil {
				logger.Info("failed to list services from project id", "error", err)
				return ctrl.Result{}, err
			}
			for _, serviceId := range servicesIds {
				r.ProcessIpAuthorizationForOneService(ctx, crd, *nodes, crd.Spec.ProjectId, serviceId)
			}
		} else {
			if err := r.ProcessIpAuthorizationForOneService(ctx, crd, *nodes, crd.Spec.ProjectId, crd.Spec.ServiceId); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	return ctrl.Result{}, nil
}

func (r *ServiceReconciler) ProcessIpAuthorizationForOneService(ctx context.Context, crd v1alpha1.Service, nodes corev1.NodeList, projectId string, serviceId string) error {

	ips, engine, err := ListAuthorizedIps(ctx, r, projectId, serviceId)
	if err != nil {
		r.Log.Info("failed to list nodes", "error", err)
		return err
	}

	nodesMap := make(map[string]*corev1.Node)
	for _, node := range nodes.Items {
		for k, v := range crd.Spec.LabelSelector.MatchLabels {
			if node.Labels[k] == v {
				nodesMap[getExternalAddress(node)] = &node
				break
			}
		}
	}

	for _, ip := range ips {
		if nodesMap[ip] != nil {
			description := fmt.Sprintf("K8S-CDB-Operator_%s_%s_%s", nodesMap[ip].Name, crd.UID, nodesMap[ip].UID)
			if err := AuthorizeNodeIp(ctx, r, projectId, serviceId, engine, ip, description); err != nil {
				return err
			}
		} else {
			if err := UnauthorizeNodeIp(ctx, r, projectId, serviceId, engine, ip); err != nil {
				return err
			}
		}
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {

	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.Service{}).
		For(&corev1.Node{}).
		Complete(r)
}

func getExternalAddress(node corev1.Node) string {
	for _, address := range node.Status.Addresses {
		if address.Type == "ExternalIP" {
			return address.Address
		}
	}
	return ""
}
