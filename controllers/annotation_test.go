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
	"reflect"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/ovh/public-cloud-databases-operator/api/v1alpha1"
)

func databaseWithAnnotation(value string) v1alpha1.Database {
	crd := v1alpha1.Database{ObjectMeta: metav1.ObjectMeta{Name: "db", UID: "uid-1234"}}
	if value != "" {
		crd.Annotations = map[string]string{additionalIpsAnnotation: value}
	}
	return crd
}

func TestGetAnnotationAddresses(t *testing.T) {
	nodeIP := IpRestriction{IP: "10.0.0.1/32", Description: "K8S-CDB-Operator_node1_uid-1234_nodeuid"}
	description := "K8S-CDB-Operator_annotation_uid-1234"

	tests := []struct {
		name       string
		annotation string
		existing   []IpRestriction
		want       []IpRestriction
	}{
		{
			name:       "no annotation leaves the list untouched",
			annotation: "",
			existing:   []IpRestriction{nodeIP},
			want:       []IpRestriction{nodeIP},
		},
		{
			name:       "bare IP becomes a single host block",
			annotation: "203.0.113.5",
			existing:   []IpRestriction{nodeIP},
			want: []IpRestriction{
				nodeIP,
				{IP: "203.0.113.5/32", Description: description},
			},
		},
		{
			name:       "CIDR block is kept and normalized",
			annotation: "192.0.2.10/24",
			existing:   nil,
			want:       []IpRestriction{{IP: "192.0.2.0/24", Description: description}},
		},
		{
			name:       "IPv6 gets a /128 mask",
			annotation: "2001:db8::1",
			existing:   nil,
			want:       []IpRestriction{{IP: "2001:db8::1/128", Description: description}},
		},
		{
			name:       "entries are split on commas and whitespace",
			annotation: "203.0.113.5, 203.0.113.6\n198.51.100.0/24",
			existing:   nil,
			want: []IpRestriction{
				{IP: "203.0.113.5/32", Description: description},
				{IP: "203.0.113.6/32", Description: description},
				{IP: "198.51.100.0/24", Description: description},
			},
		},
		{
			name:       "invalid entries are skipped, valid ones kept",
			annotation: "not-an-ip, 203.0.113.5, 10.0.0.0/99",
			existing:   nil,
			want:       []IpRestriction{{IP: "203.0.113.5/32", Description: description}},
		},
		{
			name:       "IP already discovered from a node is not duplicated",
			annotation: "10.0.0.1, 203.0.113.5",
			existing:   []IpRestriction{nodeIP},
			want: []IpRestriction{
				nodeIP,
				{IP: "203.0.113.5/32", Description: description},
			},
		},
		{
			name:       "IP repeated inside the annotation is not duplicated",
			annotation: "203.0.113.5, 203.0.113.5/32",
			existing:   nil,
			want:       []IpRestriction{{IP: "203.0.113.5/32", Description: description}},
		},
		{
			name:       "annotation set but empty adds nothing",
			annotation: "  ,  ",
			existing:   []IpRestriction{nodeIP},
			want:       []IpRestriction{nodeIP},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getAnnotationAddresses(context.Background(), databaseWithAnnotation(tt.annotation), tt.existing)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getAnnotationAddresses() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// The description must carry the operator prefix and the CRD UID, otherwise the
// reconciliation and the deletion cleanup would treat the entry as a foreign one
// and never refresh or remove it.
func TestAnnotationIpRestrictionDescriptionIsOwned(t *testing.T) {
	crd := databaseWithAnnotation("203.0.113.5")
	got := getAnnotationAddresses(context.Background(), crd, nil)

	if len(got) != 1 {
		t.Fatalf("expected 1 ip restriction, got %d", len(got))
	}

	description := got[0].Description
	if !isOwnedDescription(description, crd) {
		t.Errorf("description %q is not recognized as owned by the CRD", description)
	}
}
