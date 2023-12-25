package controllers

import (
	"context"
	"fmt"
)

type IpRestriction struct {
	IP          string `json:"ip"`
	Description string `json:"description"`
}
type Cluster struct {
	ID          string          `json:"id"`
	Engine      string          `json:"engine"`
	Ips         []IpRestriction `json:"ipRestrictions"`
	NetworkType string          `json:"networkType"`
}
type ClusterUpdate struct {
	Ips []IpRestriction `json:"ipRestrictions"`
}

const (
	PrefixEndpoint     = "/cloud/project"
	GetServiceEndpoint = "database/service"
	Mask               = "/32"
)

func GetServicesForProjectId(ctx context.Context, r *DatabaseReconciler, projectId string) ([]string, error) {
	response := []string{}
	endpoint := fmt.Sprintf("%s/%s/%s", PrefixEndpoint, projectId, GetServiceEndpoint)

	return response, r.OvhClient.Get(endpoint, &response)
}

func GetCluster(ctx context.Context, r *DatabaseReconciler, projectId string, serviceId string) (*Cluster, error) {
	response := Cluster{}
	endpoint := fmt.Sprintf("%s/%s/%s/%s", PrefixEndpoint, projectId, GetServiceEndpoint, serviceId)

	return &response, r.OvhClient.Get(endpoint, &response)
}

func UpdateClusterNodeIps(ctx context.Context, r *DatabaseReconciler, projectId string, serviceId string, engine string, ips []IpRestriction) error {
	endpoint := fmt.Sprintf("%s/%s/database/%s/%s", PrefixEndpoint, projectId, engine, serviceId)

	return r.OvhClient.Put(endpoint, ClusterUpdate{Ips: ips}, nil)
}
