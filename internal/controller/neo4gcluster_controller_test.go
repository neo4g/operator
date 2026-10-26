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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
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
						Image:    "ghcr.io/neo4g/neo4g:latest",
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

			By("exposing HTTP for clients and gRPC for internal traffic")
			var sts appsv1.StatefulSet
			Expect(k8sClient.Get(ctx, typeNamespacedName, &sts)).To(Succeed())
			Expect(envValue(sts.Spec.Template.Spec.Containers[0].Env, "NEO4G_LISTEN_ADDR")).To(Equal(":7474"))
			Expect(envValue(sts.Spec.Template.Spec.Containers[0].Env, "NEO4G_GRPC_ADDR")).To(Equal(":9474"))
			Expect(envValue(sts.Spec.Template.Spec.Containers[0].Env, "NEO4G_PRIMARY_ADDR")).To(Equal(
				"test-cluster-0.test-cluster-headless.default.svc:9474",
			))

			var headless corev1.Service
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: resourceName + "-headless", Namespace: "default",
			}, &headless)).To(Succeed())
			Expect(servicePort(headless.Spec.Ports, "neo4g")).To(Equal(int32(7474)))
			Expect(servicePort(headless.Spec.Ports, "grpc")).To(Equal(int32(9474)))

			var clientSvc corev1.Service
			Expect(k8sClient.Get(ctx, typeNamespacedName, &clientSvc)).To(Succeed())
			Expect(clientSvc.Spec.Ports).To(HaveLen(1))
			Expect(clientSvc.Spec.Ports[0].Port).To(Equal(int32(7474)))

			By("pointing the gateway at node gRPC addresses")
			cr := &neo4gv1alpha1.Neo4gCluster{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, cr)).To(Succeed())
			cr.Spec.Replicas = 3
			Expect(k8sClient.Update(ctx, cr)).To(Succeed())
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			var deploy appsv1.Deployment
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: resourceName + "-gateway", Namespace: "default",
			}, &deploy)).To(Succeed())
			nodes := envValue(deploy.Spec.Template.Spec.Containers[0].Env, "NEO4G_GW_NODES")
			Expect(nodes).To(Equal(
				"test-cluster-0=test-cluster-0.test-cluster-headless.default.svc:9474," +
					"test-cluster-1=test-cluster-1.test-cluster-headless.default.svc:9474," +
					"test-cluster-2=test-cluster-2.test-cluster-headless.default.svc:9474",
			))
			Expect(nodes).NotTo(ContainSubstring("http://"))
		})
	})
})

func envValue(env []corev1.EnvVar, name string) string {
	for _, e := range env {
		if e.Name == name {
			return e.Value
		}
	}
	return ""
}

func servicePort(ports []corev1.ServicePort, name string) int32 {
	for _, p := range ports {
		if p.Name == name {
			return p.Port
		}
	}
	return 0
}
