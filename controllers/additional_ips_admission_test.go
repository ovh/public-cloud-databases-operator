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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	cloudv1alpha1 "github.com/ovh/public-cloud-databases-operator/api/v1alpha1"
)

// Exercises the generated CRD against a real API server, so the pattern is checked by
// the code that will actually enforce it rather than by our own regexp engine.
var _ = Describe("spec.additionalIps", func() {
	It("accepts valid IPs and CIDR blocks and rejects everything else", func() {
		accepted := []string{
			"0.0.0.0", "1.2.3.4", "203.0.113.5", "255.255.255.255",
			"0.0.0.0/0", "10.0.0.0/8", "192.0.2.10/24", "10.0.0.1/32",
			"::", "::1", "2001:db8::1", "fe80::1", "1:2:3:4:5:6:7:8",
			"::ffff:192.168.1.1", "64:ff9b::1.2.3.4",
			"::/0", "2001:db8::/32", "2001:db8::1/128", "fe80::/10",
		}
		rejected := []string{
			// shaped like an IP but not one
			"abc", "deadbeef", "1.2.3", "...", "a.b.c.d",
			// out of range
			"256.1.1.1", "999.999.999.999", "1.2.3.4.5", "10.0.0.0/999",
			"1.2.3.4/33", "2001:db8::/129", "::/999",
			// malformed
			"010.0.0.1", "1:2:3:4:5:6:7:8:9", "1:::2", "12345::1", "gggg::1",
			"fe80::1%eth0", "", " ", "1.2.3.4 ", "1.2.3.4/", "/32",
		}

		create := func(entry string) error {
			return k8sClient.Create(context.Background(), &cloudv1alpha1.Database{
				ObjectMeta: metav1.ObjectMeta{GenerateName: "validation-", Namespace: "default"},
				Spec: cloudv1alpha1.DatabaseSpec{
					ProjectId:     "project",
					AdditionalIps: []string{entry},
				},
			})
		}

		for _, entry := range accepted {
			Expect(create(entry)).To(Succeed(), fmt.Sprintf("%q should be accepted", entry))
		}
		for _, entry := range rejected {
			Expect(create(entry)).NotTo(Succeed(), fmt.Sprintf("%q should be rejected", entry))
		}
	})
})
