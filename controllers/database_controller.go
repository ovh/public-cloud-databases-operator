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
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/go-logr/logr"
	"github.com/ovh/go-ovh/ovh"
	"github.com/ovh/public-cloud-databases-operator/api/v1alpha1"
)

// DatabaseReconciler reconciles a Database object
type DatabaseReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	OvhClient *ovh.Client
}

//+kubebuilder:rbac:groups=cloud.ovh.net,resources=databases,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cloud.ovh.net,resources=databases/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=cloud.ovh.net,resources=databases/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Database object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.1/pkg/reconcile
func (r *DatabaseReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := ctrl.Log.WithName("controllers").WithName("Service").WithValues("req", req)
	logger.V(1).Info("reconcile")

	//v1alpha1.SchemeBuilder.
	serviceList := &v1alpha1.DatabaseList{}
	err := r.List(ctx, serviceList)
	if err != nil {
		logger.Error(err, "failed to list crd")
		return ctrl.Result{}, err
	}
	for _, crd := range serviceList.Items {
		logger.V(1).Info(fmt.Sprintf("spec: %v", crd.Spec))
		logger := logger.WithValues("project_id", crd.Spec.ProjectId)
		opts := []client.ListOption{}
		if crd.Spec.LabelSelector != nil {
			logger.V(1).Info(fmt.Sprintf("match labels: %v", crd.Spec.LabelSelector.MatchLabels))
			for k, v := range crd.Spec.LabelSelector.MatchLabels {
				opts = append(opts, client.MatchingLabels{k: v})
			}
		}

		nodes := &corev1.NodeList{}
		logger.V(1).Info(fmt.Sprintf("opts: %v", opts))
		err = r.List(ctx, nodes, opts...)

		if err != nil {
			logger.Error(err, "failed to list nodes")
			return ctrl.Result{}, err
		}
		logger.Info(fmt.Sprintf("nodes count: %v", len(nodes.Items)))

		// check if there is a wildcard on service id, then process on all the services of the project
		if crd.Spec.ServiceId == "" {
			servicesIds, err := GetServicesForProjectId(ctx, r, crd.Spec.ProjectId)
			logger.Info(fmt.Sprintf("serviceIDs %v", servicesIds))
			if err != nil {
				logger.Error(err, "failed to list services from project id")
				return ctrl.Result{}, err
			}
			for _, serviceId := range servicesIds {
				logger := logger.WithValues("service_id", serviceId)
				logger.V(1).Info("processing")
				if err := r.ProcessIpAuthorizationForOneService(logr.NewContext(ctx, logger), crd, *nodes, crd.Spec.ProjectId, serviceId); err != nil {
					logger.Error(err, "failed to process ip authorization")
					return ctrl.Result{}, err
				}
				logger.V(1).Info("done processing")
			}
		} else {
			logger := logger.WithValues("service_id", crd.Spec.ServiceId)
			logger.V(1).Info("processing")
			if err := r.ProcessIpAuthorizationForOneService(logr.NewContext(ctx, logger), crd, *nodes, crd.Spec.ProjectId, crd.Spec.ServiceId); err != nil {
				logger.Error(err, "failed to process ip authorization")
				return ctrl.Result{}, err
			}
			logger.V(1).Info("done processing")
		}
	}

	return ctrl.Result{}, nil
}

func (r *DatabaseReconciler) ProcessIpAuthorizationForOneService(ctx context.Context, crd v1alpha1.Database, nodes corev1.NodeList, projectId string, serviceId string) error {
	logger := logr.FromContextOrDiscard(ctx)
	cluster, err := GetCluster(ctx, r, projectId, serviceId)
	if err != nil {
		return err
	}
	logger.V(1).Info(fmt.Sprintf("Old IPs: %+v", cluster.Ips))

	newIPs := make([]IpRestriction, 0)
	nodesMap := make(map[string]struct{})
	for _, node := range nodes.Items {
		ip := getInternalAddress(node) + Mask
		nodesMap[ip] = struct{}{}
		newIPs = append(newIPs, IpRestriction{IP: ip, Description: fmt.Sprintf("K8S-CDB-Operator_%s_%s_%s", node.Name, crd.UID, node.UID)})
	}

	for _, ip := range cluster.Ips {
		_, exist := nodesMap[ip.IP]
		if !exist && !strings.HasPrefix(ip.Description, "K8S-CDB-Operator_") {
			newIPs = append(newIPs, ip)
		}
	}

	logger.V(1).Info(fmt.Sprintf("New IPs: %+v", newIPs))
	return UpdateClusterNodeIps(ctx, r, projectId, serviceId, cluster.Engine, newIPs)
}

// SetupWithManager sets up the controller with the Manager.
func (r *DatabaseReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.Database{}).
		Owns(&corev1.Node{}).
		Complete(r)
}

func getInternalAddress(node corev1.Node) string {
	for _, address := range node.Status.Addresses {
		if address.Type == "InternalIP" {
			return address.Address
		}
	}
	return ""
}
