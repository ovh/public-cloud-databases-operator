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
	"io"
	"net/http"
	"net/netip"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/ovh/go-ovh/ovh"
	"github.com/ovh/public-cloud-databases-operator/api/v1alpha1"
)

const finalizerName = "databases.cloud.ovh.net/ip-cleanup"

// DatabaseReconciler reconciles a Database object
type DatabaseReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	OvhClient *ovh.Client
}

//+kubebuilder:rbac:groups=cloud.ovh.net,resources=databases,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cloud.ovh.net,resources=databases/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=cloud.ovh.net,resources=databases/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=events,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.1/pkg/reconcile
func (r *DatabaseReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := ctrl.Log.WithName("controllers").WithName("Service").WithValues("req", req)
	logger.V(1).Info("reconcile")

	serviceList := &v1alpha1.DatabaseList{}
	err := r.List(ctx, serviceList)
	if err != nil {
		logger.Error(err, "failed to list crd")
		return ctrl.Result{}, err
	}
	for i := range serviceList.Items {
		crd := &serviceList.Items[i]
		logger.V(1).Info(fmt.Sprintf("spec: %v", crd.Spec))
		logger := logger.WithValues("project_id", crd.Spec.ProjectId)

		// Handle deletion: remove our IPs from OVH then release the finalizer
		if !crd.DeletionTimestamp.IsZero() {
			if controllerutil.ContainsFinalizer(crd, finalizerName) {
				if err := r.cleanupIpRestrictions(ctx, crd); err != nil {
					logger.Error(err, "failed to cleanup ip restrictions on deletion")
					return ctrl.Result{}, err
				}
				controllerutil.RemoveFinalizer(crd, finalizerName)
				if err := r.Update(ctx, crd); err != nil {
					return ctrl.Result{}, err
				}
			}
			continue
		}

		// Ensure finalizer is registered
		if !controllerutil.ContainsFinalizer(crd, finalizerName) {
			controllerutil.AddFinalizer(crd, finalizerName)
			if err := r.Update(ctx, crd); err != nil {
				return ctrl.Result{}, err
			}
		}

		opts := []client.ListOption{}
		if crd.Spec.LabelSelector != nil {
			logger.V(1).Info(fmt.Sprintf("match labels: %v", crd.Spec.LabelSelector.MatchLabels))
			for k, v := range crd.Spec.LabelSelector.MatchLabels {
				opts = append(opts, client.MatchingLabels{k: v})
			}
		}

		nodes := corev1.NodeList{}
		logger.V(1).Info(fmt.Sprintf("opts: %v", opts))
		err = r.List(ctx, &nodes, opts...)
		if err != nil {
			logger.Error(err, "failed to list nodes")
			return ctrl.Result{}, err
		}
		logger.Info(fmt.Sprintf("nodes count: %d", len(nodes.Items)))

		var servicesIds []string
		if crd.Spec.ServiceId == "" {
			servicesIds, err = GetServicesForProjectId(ctx, r, crd.Spec.ProjectId)
			if err != nil {
				logger.Error(err, "failed to list services from project id")
				return ctrl.Result{}, err
			}
		} else {
			servicesIds = append(servicesIds, crd.Spec.ServiceId)
		}
		for _, serviceId := range servicesIds {
			logger := logger.WithValues("service_id", serviceId)
			logger.V(1).Info("processing")
			if err := r.UpdateServiceIpRestriction(log.IntoContext(ctx, logger), *crd, nodes, crd.Spec.ProjectId, serviceId); err != nil {
				logger.Error(err, "failed to process ip restriction")
				return ctrl.Result{}, err
			}
			logger.V(1).Info("done processing")
		}
	}

	return ctrl.Result{}, nil
}

func (r *DatabaseReconciler) cleanupIpRestrictions(ctx context.Context, crd *v1alpha1.Database) error {
	logger := log.FromContext(ctx)

	var serviceIds []string
	if crd.Spec.ServiceId == "" {
		var err error
		serviceIds, err = GetServicesForProjectId(ctx, r, crd.Spec.ProjectId)
		if err != nil {
			return err
		}
	} else {
		serviceIds = []string{crd.Spec.ServiceId}
	}

	for _, serviceId := range serviceIds {
		cluster, err := GetCluster(ctx, r, crd.Spec.ProjectId, serviceId)
		if err != nil {
			return err
		}

		// no desired IP on deletion, so this keeps only what belongs to someone else
		remaining := mergeIpRestrictions(ctx, nil, cluster.Ips, *crd)

		logger.Info(fmt.Sprintf("cleanup: removing IPs for CRD %s, %d IPs remaining", crd.UID, len(remaining)))
		if err := UpdateClusterNodeIps(ctx, r, crd.Spec.ProjectId, serviceId, cluster.Engine, remaining); err != nil {
			return err
		}
	}
	return nil
}

func (r *DatabaseReconciler) UpdateServiceIpRestriction(ctx context.Context, crd v1alpha1.Database, nodes corev1.NodeList, projectId string, serviceId string) error {
	logger := log.FromContext(ctx)
	cluster, err := GetCluster(ctx, r, projectId, serviceId)
	if err != nil {
		return err
	}
	logger.V(1).Info(fmt.Sprintf("Old IPs: %+v", cluster.Ips))

	desiredIPs, err := getKubeInternalAddress(ctx, nodes, crd)
	if err != nil {
		return err
	}

	// if db is public get kube node public ip
	if cluster.NetworkType == "public" {
		desiredIPs, err = getKubePublicAddesses(ctx, nodes, crd, desiredIPs)
		if err != nil {
			return err
		}
	}

	// IPs declared on the CR are authorized on top of the ones discovered from the nodes.
	// This has to run after getKubePublicAddesses, which may reset the list to the sole GW IP.
	desiredIPs = append(desiredIPs, getAdditionalAddresses(ctx, crd)...)

	newIPs := mergeIpRestrictions(ctx, desiredIPs, cluster.Ips, crd)
	logger.V(1).Info(fmt.Sprintf("New IPs: %+v", newIPs))
	return UpdateClusterNodeIps(ctx, r, projectId, serviceId, cluster.Engine, newIPs)
}

// SetupWithManager sets up the controller with the Manager.
func (r *DatabaseReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.Database{}).
		Watches(&corev1.Node{}, handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, object client.Object) []ctrl.Request {
			databaseList := &v1alpha1.DatabaseList{}
			if err := mgr.GetClient().List(ctx, databaseList); err != nil {
				mgr.GetLogger().Error(err, "failed to list crd")
				return nil
			}

			reqs := make([]ctrl.Request, 0, len(databaseList.Items))
			for _, database := range databaseList.Items {
				reqs = append(reqs, ctrl.Request{
					NamespacedName: types.NamespacedName{
						Namespace: database.GetNamespace(),
						Name:      database.GetName(),
					},
				})
			}

			return reqs
		})).
		WithEventFilter(predicate.Funcs{
			GenericFunc: func(e event.GenericEvent) bool {
				return false
			},
		}).
		Complete(r)
}

func getKubeInternalAddress(ctx context.Context, nodes corev1.NodeList, crd v1alpha1.Database) ([]IpRestriction, error) {
	logger := log.FromContext(ctx)
	newIPs := make([]IpRestriction, 0)
	for _, node := range nodes.Items {
		for _, address := range node.Status.Addresses {
			if address.Type == "InternalIP" {
				ip := fmt.Sprintf("%s%s", address.Address, Mask)
				newIPs = append(newIPs, IpRestriction{IP: ip, Description: IpRestrictionDescription(node, crd)})
			}
		}
	}
	logger.V(1).Info(fmt.Sprintf("New IPs (Internal): %+v", newIPs))
	return newIPs, nil
}

func getKubePublicAddesses(ctx context.Context, nodes corev1.NodeList, crd v1alpha1.Database, newIPs []IpRestriction) ([]IpRestriction, error) {
	logger := log.FromContext(ctx)

	// build public ip list based on kubernetes nodes
	ipsMap := make(map[string]struct{})

	for _, ip := range newIPs {
		ipsMap[ip.IP] = struct{}{}
	}

	for _, node := range nodes.Items {
		for _, address := range node.Status.Addresses {
			if address.Type == "ExternalIP" {
				ip := fmt.Sprintf("%s%s", address.Address, Mask)
				ipsMap[ip] = struct{}{}
				newIPs = append(newIPs, IpRestriction{IP: ip, Description: IpRestrictionDescription(node, crd)})
			}
		}
	}
	logger.V(1).Info(fmt.Sprintf("New IPs (External): %+v", newIPs))

	// Get the egress ip used from the cluster (the operator is inside the cluster)
	ifconfigURL := "https://ifconfig.io"
	res, err := http.Get(ifconfigURL)
	if err != nil {
		return nil, err
	}

	resBody, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	ip := strings.TrimSpace(string(resBody))
	ipMask := fmt.Sprintf("%s%s", ip, Mask)
	logger.V(1).Info(fmt.Sprintf("Ifconfig IPs: %s", ipMask))

	// check if public ip return by ifconfig.io is one of the kubernetes node
	_, exist := ipsMap[ipMask]
	if !exist {
		// if the ip is not one of the nodes that mean the kubernetes cluster use a gateway
		// so only return gateway public ip
		newIPs = []IpRestriction{
			{
				IP:          ipMask,
				Description: fmt.Sprintf("%s_kubeGW_%s", ipRestrictionPrefix, crd.UID),
			},
		}
	}
	return newIPs, nil
}

// getAdditionalAddresses returns the IP restrictions for spec.additionalIps. It
// authorizes hosts the operator cannot discover from the Kubernetes API, such as a
// bastion, a CI runner or a VPN endpoint.
//
// The CRD pattern keeps grossly malformed values out at admission time, but it cannot
// tell a real IP from something merely shaped like one, so entries are parsed again
// here. An unparseable entry is logged and skipped rather than failing the
// reconciliation: a typo on one CR must not stop the others from being processed.
//
// Duplicates are not filtered here, mergeIpRestrictions does it for every source at once.
func getAdditionalAddresses(ctx context.Context, crd v1alpha1.Database) []IpRestriction {
	logger := log.FromContext(ctx)

	additionalIPs := make([]IpRestriction, 0, len(crd.Spec.AdditionalIps))
	for _, entry := range crd.Spec.AdditionalIps {
		ip, err := parseIpBlock(strings.TrimSpace(entry))
		if err != nil {
			logger.Error(err, "ignoring entry of spec.additionalIps")
			continue
		}
		additionalIPs = append(additionalIPs, IpRestriction{IP: ip, Description: AdditionalIpRestrictionDescription(crd)})
	}

	logger.V(1).Info(fmt.Sprintf("New IPs (Additional): %+v", additionalIPs))
	return additionalIPs
}

// mergeIpRestrictions builds the ipRestrictions payload for one service out of the IPs
// this CR wants authorized and the IPs already on the service.
//
// The whole list is sent on every write and the API rejects a payload carrying the same
// IP twice, so the merge deduplicates by IP across every source at once: node to node
// (two nodes can share an egress IP), node to spec.additionalIps, and ours to an entry
// already on the service.
//
// Entries the operator did not create for this CR are kept untouched, and they win over
// a desired IP that collides with them. The IP ends up authorized either way, so keeping
// the existing description means the operator neither takes over a restriction it did
// not create, whether it came from the console or from another Database CR targeting the
// same service, nor deletes it when this CR goes away.
func mergeIpRestrictions(ctx context.Context, desired []IpRestriction, existing []IpRestriction, crd v1alpha1.Database) []IpRestriction {
	logger := log.FromContext(ctx)

	seen := make(map[string]struct{}, len(existing)+len(desired))

	foreign := make([]IpRestriction, 0, len(existing))
	for _, ip := range existing {
		// our own entries are rebuilt from scratch on every reconciliation
		if isOwnedDescription(ip.Description, crd) {
			continue
		}
		if _, duplicate := seen[ip.IP]; duplicate {
			continue
		}
		seen[ip.IP] = struct{}{}
		foreign = append(foreign, ip)
	}

	merged := make([]IpRestriction, 0, len(desired)+len(foreign))
	for _, ip := range desired {
		if _, duplicate := seen[ip.IP]; duplicate {
			logger.V(1).Info(fmt.Sprintf("IP %s is already authorized, not adding %s", ip.IP, ip.Description))
			continue
		}
		seen[ip.IP] = struct{}{}
		merged = append(merged, ip)
	}

	return append(merged, foreign...)
}

// parseIpBlock normalizes a bare IP or a CIDR block into the ipBlock form expected
// by the OVHcloud API. A bare IP is turned into a single host block.
func parseIpBlock(entry string) (string, error) {
	if strings.Contains(entry, "/") {
		prefix, err := netip.ParsePrefix(entry)
		if err != nil {
			return "", fmt.Errorf("invalid CIDR block %q: %w", entry, err)
		}
		return prefix.Masked().String(), nil
	}

	addr, err := netip.ParseAddr(entry)
	if err != nil {
		return "", fmt.Errorf("invalid IP address %q: %w", entry, err)
	}
	// netip.PrefixFrom would silently drop a zone, and netip.ParsePrefix rejects one
	// outright, so refuse it here too: a zone is meaningless in an ip restriction.
	if addr.Zone() != "" {
		return "", fmt.Errorf("invalid IP address %q: zone identifiers are not supported", entry)
	}
	return netip.PrefixFrom(addr, addr.BitLen()).String(), nil
}

const ipRestrictionPrefix = "K8S-CDB-Operator"

func IpRestrictionDescription(node corev1.Node, crd v1alpha1.Database) string {
	return fmt.Sprintf("%s_%s_%s_%s", ipRestrictionPrefix, node.Name, crd.UID, node.UID)
}

// isOwnedDescription reports whether an existing ip restriction was created by this
// operator for this very CRD, and may therefore be refreshed or removed by it.
// Entries belonging to another CRD, another cluster or to the user are left alone.
func isOwnedDescription(description string, crd v1alpha1.Database) bool {
	return strings.HasPrefix(description, ipRestrictionPrefix) && strings.Contains(description, string(crd.UID))
}

func AdditionalIpRestrictionDescription(crd v1alpha1.Database) string {
	return fmt.Sprintf("%s_additionalIp_%s", ipRestrictionPrefix, crd.UID)
}
