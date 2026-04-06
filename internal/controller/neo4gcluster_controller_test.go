/*
Copyright 2026.

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

package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	neo4gv1alpha1 "github.com/neo4g/operator/api/v1alpha1"
)

var _ = Describe("Neo4gCluster Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-cluster"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}
		neo4gcluster := &neo4gv1alpha1.Neo4gCluster{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind Neo4gCluster")
			err := k8sClient.Get(ctx, typeNamespacedName, neo4gcluster)
			if err != nil && errors.IsNotFound(err) {
				cr := &neo4gv1alpha1.Neo4gCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: neo4gv1alpha1.Neo4gClusterSpec{
						Replicas: 1,
						Image:    "ghcr.io/seankohjs/neo4g:latest",
					},
				}
				Expect(k8sClient.Create(ctx, cr)).To(Succeed())
			}
		})

		AfterEach(func() {
			cr := &neo4gv1alpha1.Neo4gCluster{}
			err := k8sClient.Get(ctx, typeNamespacedName, cr)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance Neo4gCluster")
			Expect(k8sClient.Delete(ctx, cr)).To(Succeed())
		})

		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &Neo4gClusterReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
