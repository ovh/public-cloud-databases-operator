package controllers

import (
	"context"
	"fmt"
)

type IpRestriction struct {
	IP          string `json:"ip"`
	Description string `json:"description"`
}
type Resp struct {
	Engine string          `json:"engine"`
	Ips    []IpRestriction `json:"ipRestrictions"`
}

const (
	PrefixEndpoint     = "/cloud/project"
	GetServiceEndpoint = "database/service"
)

func AuthorizeNodeIp(ctx context.Context, r *ServiceReconciler, projectId string, serviceId string, engine string, ip string, description string) error {
	endpoint := fmt.Sprintf("%s/%s/database/%s/%s/ipRestriction", PrefixEndpoint, projectId, engine, serviceId)
	req := IpRestriction{
		IP:          ip,
		Description: description,
	}
	return r.OvhClient.Post(endpoint, req, nil)
}
func UnauthorizeNodeIp(ctx context.Context, r *ServiceReconciler, projectId string, serviceId string, engine string, ip string) error {
	endpoint := fmt.Sprintf("%s/%s/database/%s/%s/ipRestriction/%s", PrefixEndpoint, projectId, engine, serviceId, ip)
	return r.OvhClient.Delete(endpoint, nil)
}
func ListAuthorizedIps(ctx context.Context, r *ServiceReconciler, projectId string, serviceId string) ([]string, string, error) {
	var response Resp
	ips := make([]string, 0)

	endpoint := fmt.Sprintf("%s/%s/%s/%s", PrefixEndpoint, projectId, GetServiceEndpoint, serviceId)
	if err := r.OvhClient.Get(endpoint, response); err != nil {
		return ips, "", err
	}

	for _, ip := range response.Ips {
		ips = append(ips, ip.IP)
	}
	return ips, response.Engine, nil
}

func GetServicesForProjectId(ctx context.Context, r *ServiceReconciler, projectId string) ([]string, error) {
	var response []string
	endpoint := fmt.Sprintf("%s/%s/%s", PrefixEndpoint, projectId, GetServiceEndpoint)

	if err := r.OvhClient.Get(endpoint, response); err != nil {
		return []string{}, err
	}
	return response, nil
}
