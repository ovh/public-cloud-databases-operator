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
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"

	"github.com/ovh/public-cloud-databases-operator/api/v1alpha1"
)

func databaseWithAdditionalIps(ips ...string) v1alpha1.Database {
	return v1alpha1.Database{
		ObjectMeta: metav1.ObjectMeta{Name: "db", UID: "uid-1234"},
		Spec:       v1alpha1.DatabaseSpec{AdditionalIps: ips},
	}
}

func TestGetAdditionalAddresses(t *testing.T) {
	description := "K8S-CDB-Operator_additionalIp_uid-1234"

	tests := []struct {
		name string
		spec []string
		want []IpRestriction
	}{
		{
			name: "empty additionalIps yields nothing",
			spec: nil,
			want: []IpRestriction{},
		},
		{
			name: "bare IP becomes a single host block",
			spec: []string{"203.0.113.5"},
			want: []IpRestriction{{IP: "203.0.113.5/32", Description: description}},
		},
		{
			name: "CIDR block is kept and normalized",
			spec: []string{"192.0.2.10/24"},
			want: []IpRestriction{{IP: "192.0.2.0/24", Description: description}},
		},
		{
			name: "IPv6 gets a /128 mask",
			spec: []string{"2001:db8::1"},
			want: []IpRestriction{{IP: "2001:db8::1/128", Description: description}},
		},
		{
			name: "order of the spec list is preserved",
			spec: []string{"203.0.113.5", "203.0.113.6", "198.51.100.0/24"},
			want: []IpRestriction{
				{IP: "203.0.113.5/32", Description: description},
				{IP: "203.0.113.6/32", Description: description},
				{IP: "198.51.100.0/24", Description: description},
			},
		},
		{
			name: "surrounding whitespace is tolerated",
			spec: []string{"  203.0.113.5  "},
			want: []IpRestriction{{IP: "203.0.113.5/32", Description: description}},
		},
		{
			// the CRD pattern rejects most of these at admission, but values that
			// match the pattern without being real IPs still reach the controller
			name: "invalid entries are skipped, valid ones kept",
			spec: []string{"1.2.3", "203.0.113.5", "10.0.0.0/99", ":::"},
			want: []IpRestriction{{IP: "203.0.113.5/32", Description: description}},
		},
		{
			name: "list of only blank entries yields nothing",
			spec: []string{"", "   "},
			want: []IpRestriction{},
		},
		{
			// deduplication is mergeIpRestrictions' job, not this function's
			name: "repeated entries are passed through untouched",
			spec: []string{"203.0.113.5", "203.0.113.5/32"},
			want: []IpRestriction{
				{IP: "203.0.113.5/32", Description: description},
				{IP: "203.0.113.5/32", Description: description},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getAdditionalAddresses(context.Background(), databaseWithAdditionalIps(tt.spec...))
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getAdditionalAddresses() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestMergeIpRestrictions(t *testing.T) {
	crd := databaseWithAdditionalIps()

	ours := func(ip, suffix string) IpRestriction {
		return IpRestriction{IP: ip, Description: "K8S-CDB-Operator_" + suffix + "_uid-1234"}
	}
	otherCRD := IpRestriction{IP: "10.0.0.1/32", Description: "K8S-CDB-Operator_node1_uid-9999_nodeuid"}
	console := IpRestriction{IP: "203.0.113.5/32", Description: "office uplink"}

	tests := []struct {
		name     string
		desired  []IpRestriction
		existing []IpRestriction
		want     []IpRestriction
	}{
		{
			name:     "nothing on the service keeps the desired list as it is",
			desired:  []IpRestriction{ours("10.0.0.1/32", "node1")},
			existing: nil,
			want:     []IpRestriction{ours("10.0.0.1/32", "node1")},
		},
		{
			name:     "our stale entries are dropped and rebuilt",
			desired:  []IpRestriction{ours("10.0.0.2/32", "node2")},
			existing: []IpRestriction{ours("10.0.0.1/32", "node1")},
			want:     []IpRestriction{ours("10.0.0.2/32", "node2")},
		},
		{
			name:     "foreign entries are preserved alongside ours",
			desired:  []IpRestriction{ours("10.0.0.2/32", "node2")},
			existing: []IpRestriction{console},
			want:     []IpRestriction{ours("10.0.0.2/32", "node2"), console},
		},
		{
			// the reported bug: a console entry for an IP we also want to authorize
			name:     "a console entry wins over the desired duplicate",
			desired:  []IpRestriction{ours("203.0.113.5/32", "additionalIp")},
			existing: []IpRestriction{console},
			want:     []IpRestriction{console},
		},
		{
			// two clusters behind the same egress gateway, or two CRs on one service
			name:     "another CR's entry wins over the desired duplicate",
			desired:  []IpRestriction{ours("10.0.0.1/32", "node1")},
			existing: []IpRestriction{otherCRD},
			want:     []IpRestriction{otherCRD},
		},
		{
			name:     "two nodes sharing an egress IP are collapsed",
			desired:  []IpRestriction{ours("57.130.11.110/32", "node1"), ours("57.130.11.110/32", "node2")},
			existing: nil,
			want:     []IpRestriction{ours("57.130.11.110/32", "node1")},
		},
		{
			name:     "a node IP repeated in additionalIps is collapsed",
			desired:  []IpRestriction{ours("10.0.0.1/32", "node1"), ours("10.0.0.1/32", "additionalIp")},
			existing: nil,
			want:     []IpRestriction{ours("10.0.0.1/32", "node1")},
		},
		{
			name:     "a duplicate already on the service is collapsed too",
			desired:  nil,
			existing: []IpRestriction{console, {IP: console.IP, Description: "someone else"}},
			want:     []IpRestriction{console},
		},
		{
			name:     "everything collides and only the foreign entries survive",
			desired:  []IpRestriction{ours("203.0.113.5/32", "additionalIp"), ours("10.0.0.1/32", "node1")},
			existing: []IpRestriction{console, otherCRD, ours("10.0.0.9/32", "gone")},
			want:     []IpRestriction{console, otherCRD},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeIpRestrictions(context.Background(), tt.desired, tt.existing, crd)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("mergeIpRestrictions() = %+v, want %+v", got, tt.want)
			}
			seen := map[string]int{}
			for _, ip := range got {
				seen[ip.IP]++
				if seen[ip.IP] > 1 {
					t.Errorf("IP %s appears %d times in the payload", ip.IP, seen[ip.IP])
				}
			}
		})
	}
}

// The description must carry the operator prefix and the CRD UID, otherwise the
// reconciliation and the deletion cleanup would treat the entry as a foreign one
// and never refresh or remove it.
func TestAdditionalIpRestrictionDescriptionIsOwned(t *testing.T) {
	crd := databaseWithAdditionalIps("203.0.113.5")
	got := getAdditionalAddresses(context.Background(), crd)

	if len(got) != 1 {
		t.Fatalf("expected 1 ip restriction, got %d", len(got))
	}

	description := got[0].Description
	if !isOwnedDescription(description, crd) {
		t.Errorf("description %q is not recognized as owned by the CRD", description)
	}
}

// additionalIpsPattern reads the validation pattern out of the generated CRD, so the
// marker in api/v1alpha1/database_types.go is what actually gets exercised here.
func additionalIpsPattern(t *testing.T) string {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join("..", "config", "crd", "bases", "cloud.ovh.net_databases.yaml"))
	if err != nil {
		t.Fatalf("reading generated CRD: %v", err)
	}

	var crd struct {
		Spec struct {
			Versions []struct {
				Schema struct {
					OpenAPIV3Schema struct {
						Properties struct {
							Spec struct {
								Properties struct {
									AdditionalIps struct {
										Items struct {
											Pattern string `json:"pattern"`
										} `json:"items"`
									} `json:"additionalIps"`
								} `json:"properties"`
							} `json:"spec"`
						} `json:"properties"`
					} `json:"openAPIV3Schema"`
				} `json:"schema"`
			} `json:"versions"`
		} `json:"spec"`
	}
	if err := yaml.Unmarshal(raw, &crd); err != nil {
		t.Fatalf("parsing generated CRD: %v", err)
	}
	if len(crd.Spec.Versions) == 0 {
		t.Fatal("generated CRD has no versions")
	}

	pattern := crd.Spec.Versions[0].Schema.OpenAPIV3Schema.Properties.Spec.Properties.AdditionalIps.Items.Pattern
	if pattern == "" {
		t.Fatal("generated CRD has no pattern on spec.additionalIps items")
	}
	return pattern
}

// The CRD pattern and parseIpBlock must agree. If the pattern were looser, invalid
// values would reach the OVHcloud API inside a bulk ipRestrictions payload and fail the
// whole write; if it were stricter, valid entries would be rejected at apply time.
func TestAdditionalIpsPatternMatchesParser(t *testing.T) {
	pattern := additionalIpsPattern(t)

	// the API server compiles patterns with RE2, which has no lookahead or backreference
	re, err := regexp.Compile(pattern)
	if err != nil {
		t.Fatalf("pattern does not compile under RE2: %v", err)
	}

	corpus := []string{
		// IPv4, bare and with a prefix
		"0.0.0.0", "1.2.3.4", "10.0.0.1", "203.0.113.5", "255.255.255.255",
		"0.0.0.0/0", "10.0.0.0/8", "192.0.2.10/24", "10.0.0.1/32", "172.16.0.0/12",
		// IPv6 in its various forms
		"::", "::1", "2001:db8::1", "fe80::1", "1:2:3:4:5:6:7:8", "1::8", "1::",
		"1:2:3:4:5:6:7::", "2001:0db8:0000:0000:0000:0000:0000:0001",
		"::ffff:192.168.1.1", "64:ff9b::1.2.3.4",
		"::/0", "2001:db8::/32", "2001:db8::1/128", "fe80::/10", "::1/128",
		// shaped like an IP but not one - these are what the loose pattern let through
		"abc", "deadbeef", "1.2.3", "...", "a.b.c.d", "10.0.0.0/999",
		// out-of-range octets and prefixes
		"256.1.1.1", "999.999.999.999", "1.2.3.4.5", "1.2.3.",
		"1.2.3.4/33", "2001:db8::/129", "::/999", "2001:db8::1/032",
		// leading zeros, malformed IPv6, zones, whitespace
		"010.0.0.1", "1:2:3:4:5:6:7:8:9", "1:::2", "12345::1", "gggg::1", ":",
		"fe80::1%eth0", "1.2.3.4%eth0", "", " ", "1.2.3.4 ", "1.2.3.4/", "/32",
	}
	for b := 0; b <= 40; b++ {
		corpus = append(corpus, fmt.Sprintf("10.0.0.0/%d", b))
	}
	for b := 0; b <= 136; b++ {
		corpus = append(corpus, fmt.Sprintf("2001:db8::/%d", b))
	}
	for o := 0; o <= 300; o++ {
		corpus = append(corpus, fmt.Sprintf("%d.0.0.1", o))
	}

	for _, entry := range corpus {
		_, err := parseIpBlock(entry)
		accepted, parsed := re.MatchString(entry), err == nil
		if accepted != parsed {
			t.Errorf("%q: CRD pattern accepts=%v but parseIpBlock succeeds=%v", entry, accepted, parsed)
		}
	}
}
