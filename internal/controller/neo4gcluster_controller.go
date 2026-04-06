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
	"fmt"
	"strconv"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	neo4gv1alpha1 "github.com/neo4g/operator/api/v1alpha1"
)

const (
	neo4gPort    = 7474
	gatewayPort  = 7480
	dataDir      = "/data"
	labelCluster = "neo4g.io/cluster"
	labelRole    = "neo4g.io/role"
	labelComp    = "neo4g.io/component"
)

// Neo4gClusterReconciler reconciles a Neo4gCluster object.
type Neo4gClusterReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=neo4g.neo4g.io,resources=neo4gclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=neo4g.neo4g.io,resources=neo4gclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=neo4g.neo4g.io,resources=neo4gclusters/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch

func (r *Neo4gClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	cluster := &neo4gv1alpha1.Neo4gCluster{}
	if err := r.Get(ctx, req.NamespacedName, cluster); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	log.Info("reconciling Neo4gCluster", "name", cluster.Name)

	if err := r.reconcileHeadlessService(ctx, cluster); err != nil {
		return ctrl.Result{}, fmt.Errorf("headless service: %w", err)
	}

	if err := r.reconcileStatefulSet(ctx, cluster); err != nil {
		return ctrl.Result{}, fmt.Errorf("statefulset: %w", err)
	}

	needsGateway := cluster.Spec.Replicas > 1

	if needsGateway {
		if err := r.reconcileGatewayDeployment(ctx, cluster); err != nil {
			return ctrl.Result{}, fmt.Errorf("gateway deployment: %w", err)
		}
	} else {
		if err := r.deleteIfExists(ctx, &appsv1.Deployment{}, types.NamespacedName{
			Name: cluster.Name + "-gateway", Namespace: cluster.Namespace,
		}); err != nil {
			return ctrl.Result{}, fmt.Errorf("cleanup gateway deployment: %w", err)
		}
	}

	if err := r.reconcileClientService(ctx, cluster, needsGateway); err != nil {
		return ctrl.Result{}, fmt.Errorf("client service: %w", err)
	}

	if err := r.reconcileStatus(ctx, cluster); err != nil {
		return ctrl.Result{}, fmt.Errorf("status: %w", err)
	}

	return ctrl.Result{}, nil
}

// --- Headless Service (for StatefulSet pod DNS) ---

func (r *Neo4gClusterReconciler) reconcileHeadlessService(ctx context.Context, c *neo4gv1alpha1.Neo4gCluster) error {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      c.Name + "-headless",
			Namespace: c.Namespace,
		},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, svc, func() error {
		if err := ctrl.SetControllerReference(c, svc, r.Scheme); err != nil {
			return err
		}
		svc.Labels = baseLabels(c)
		svc.Spec = corev1.ServiceSpec{
			ClusterIP: corev1.ClusterIPNone,
			Selector:  baseLabels(c),
			Ports: []corev1.ServicePort{{
				Name:       "neo4g",
				Port:       neo4gPort,
				TargetPort: intstr.FromInt(neo4gPort),
			}},
			PublishNotReadyAddresses: true,
		}
		return nil
	})
	return err
}

// --- StatefulSet ---

func (r *Neo4gClusterReconciler) reconcileStatefulSet(ctx context.Context, c *neo4gv1alpha1.Neo4gCluster) error {
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      c.Name,
			Namespace: c.Namespace,
		},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, sts, func() error {
		if err := ctrl.SetControllerReference(c, sts, r.Scheme); err != nil {
			return err
		}

		labels := baseLabels(c)
		sts.Labels = labels

		replicas := c.Spec.Replicas
		sts.Spec.Replicas = &replicas
		sts.Spec.ServiceName = c.Name + "-headless"
		sts.Spec.Selector = &metav1.LabelSelector{MatchLabels: labels}

		headlessSvc := c.Name + "-headless"
		primaryAddr := fmt.Sprintf("http://%s-0.%s.%s.svc:%d", c.Name, headlessSvc, c.Namespace, neo4gPort)

		env := r.buildNodeEnv(c, primaryAddr)

		image := c.Spec.Image
		if image == "" {
			image = "ghcr.io/seankohjs/neo4g:latest"
		}

		// Shell wrapper to derive role from StatefulSet ordinal:
		// pod-0 → primary, pod-N → replica
		roleScript := `ORDINAL=${HOSTNAME##*-}
if [ "$ORDINAL" = "0" ]; then
  export NEO4G_ROLE=primary
  unset NEO4G_PRIMARY_ADDR
else
  export NEO4G_ROLE=replica
fi
exec neo4g`

		container := corev1.Container{
			Name:    "neo4g",
			Image:   image,
			Command: []string{"sh", "-c", roleScript},
			Ports: []corev1.ContainerPort{{
				Name:          "neo4g",
				ContainerPort: neo4gPort,
			}},
			Env:       env,
			Resources: c.Spec.Resources,
			VolumeMounts: []corev1.VolumeMount{{
				Name:      "data",
				MountPath: dataDir,
			}},
			ReadinessProbe: &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{
					HTTPGet: &corev1.HTTPGetAction{
						Path: "/api/health",
						Port: intstr.FromInt(neo4gPort),
					},
				},
				InitialDelaySeconds: 5,
				PeriodSeconds:       5,
			},
			LivenessProbe: &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{
					HTTPGet: &corev1.HTTPGetAction{
						Path: "/api/health",
						Port: intstr.FromInt(neo4gPort),
					},
				},
				InitialDelaySeconds: 10,
				PeriodSeconds:       10,
			},
		}

		sts.Spec.Template = corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{Labels: labels},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{container},
			},
		}

		storageSize := c.Spec.Storage
		if storageSize != nil {
			sts.Spec.VolumeClaimTemplates = []corev1.PersistentVolumeClaim{{
				ObjectMeta: metav1.ObjectMeta{Name: "data"},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: storageSize.Size,
						},
					},
					StorageClassName: storageSize.StorageClassName,
				},
			}}
		} else {
			sts.Spec.VolumeClaimTemplates = []corev1.PersistentVolumeClaim{{
				ObjectMeta: metav1.ObjectMeta{Name: "data"},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: defaultStorageSize(),
						},
					},
				},
			}}
		}

		return nil
	})
	return err
}

func (r *Neo4gClusterReconciler) buildNodeEnv(c *neo4gv1alpha1.Neo4gCluster, primaryAddr string) []corev1.EnvVar {
	// Use the pod name as NODE_ID (set by downward API)
	env := []corev1.EnvVar{
		{
			Name: "NEO4G_NODE_ID",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.name"},
			},
		},
		{Name: "NEO4G_LISTEN_ADDR", Value: fmt.Sprintf(":%d", neo4gPort)},
		{Name: "NEO4G_DATA_DIR", Value: dataDir},
	}

	// Role assignment via init script: ordinal 0 = primary, rest = replica.
	// We use a shell wrapper to derive role from the hostname ordinal.
	env = append(env, corev1.EnvVar{
		Name:  "NEO4G_PRIMARY_ADDR",
		Value: primaryAddr,
	})

	cfg := c.Spec.Config
	if cfg != nil {
		if cfg.PoolSize != nil {
			env = append(env, corev1.EnvVar{Name: "NEO4G_POOL_SIZE", Value: strconv.Itoa(int(*cfg.PoolSize))})
		}
		if cfg.WALEnabled != nil && !*cfg.WALEnabled {
			env = append(env, corev1.EnvVar{Name: "NEO4G_WAL_ENABLED", Value: "false"})
		}
		if cfg.WALNoSync != nil && *cfg.WALNoSync {
			env = append(env, corev1.EnvVar{Name: "NEO4G_WAL_NOSYNC", Value: "true"})
		}
		if cfg.ReplPollInterval != nil {
			env = append(env, corev1.EnvVar{Name: "NEO4G_REPL_POLL_INTERVAL", Value: *cfg.ReplPollInterval})
		}
	}

	return env
}

// --- Gateway Deployment ---

func (r *Neo4gClusterReconciler) reconcileGatewayDeployment(ctx context.Context, c *neo4gv1alpha1.Neo4gCluster) error {
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      c.Name + "-gateway",
			Namespace: c.Namespace,
		},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, deploy, func() error {
		if err := ctrl.SetControllerReference(c, deploy, r.Scheme); err != nil {
			return err
		}

		labels := gatewayLabels(c)
		deploy.Labels = labels

		var replicas int32 = 1
		if c.Spec.Gateway != nil && c.Spec.Gateway.Replicas != nil {
			replicas = *c.Spec.Gateway.Replicas
		}
		deploy.Spec.Replicas = &replicas
		deploy.Spec.Selector = &metav1.LabelSelector{MatchLabels: labels}

		nodesList := r.buildGatewayNodeList(c)

		env := []corev1.EnvVar{
			{Name: "NEO4G_GW_LISTEN_ADDR", Value: fmt.Sprintf(":%d", gatewayPort)},
			{Name: "NEO4G_GW_NODES", Value: nodesList},
		}

		if c.Spec.Gateway != nil {
			if c.Spec.Gateway.HeartbeatInterval != nil {
				env = append(env, corev1.EnvVar{Name: "NEO4G_GW_HEARTBEAT_INTERVAL", Value: *c.Spec.Gateway.HeartbeatInterval})
			}
			if c.Spec.Gateway.HeartbeatTimeout != nil {
				env = append(env, corev1.EnvVar{Name: "NEO4G_GW_HEARTBEAT_TIMEOUT", Value: *c.Spec.Gateway.HeartbeatTimeout})
			}
			if c.Spec.Gateway.ElectionDelay != nil {
				env = append(env, corev1.EnvVar{Name: "NEO4G_GW_ELECTION_DELAY", Value: *c.Spec.Gateway.ElectionDelay})
			}
		}

		image := c.Spec.Image
		if image == "" {
			image = "ghcr.io/seankohjs/neo4g:latest"
		}

		container := corev1.Container{
			Name:    "neo4g-gateway",
			Image:   image,
			Command: []string{"neo4g-gateway"},
			Ports: []corev1.ContainerPort{{
				Name:          "gateway",
				ContainerPort: gatewayPort,
			}},
			Env: env,
			ReadinessProbe: &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{
					TCPSocket: &corev1.TCPSocketAction{
						Port: intstr.FromInt(gatewayPort),
					},
				},
				InitialDelaySeconds: 3,
				PeriodSeconds:       5,
			},
		}

		deploy.Spec.Template = corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{Labels: labels},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{container},
			},
		}
		return nil
	})
	return err
}

func (r *Neo4gClusterReconciler) buildGatewayNodeList(c *neo4gv1alpha1.Neo4gCluster) string {
	headlessSvc := c.Name + "-headless"
	var parts []string
	for i := int32(0); i < c.Spec.Replicas; i++ {
		podName := fmt.Sprintf("%s-%d", c.Name, i)
		addr := fmt.Sprintf("%s=http://%s.%s.%s.svc:%d", podName, podName, headlessSvc, c.Namespace, neo4gPort)
		parts = append(parts, addr)
	}
	return strings.Join(parts, ",")
}

// --- Stable Client Service ---
// Single entry point for downstream apps. Selector switches based on topology:
//   - replicas == 1: targets the primary pod directly
//   - replicas > 1:  targets the gateway deployment

func (r *Neo4gClusterReconciler) reconcileClientService(ctx context.Context, c *neo4gv1alpha1.Neo4gCluster, needsGateway bool) error {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      c.Name,
			Namespace: c.Namespace,
		},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, svc, func() error {
		if err := ctrl.SetControllerReference(c, svc, r.Scheme); err != nil {
			return err
		}
		svc.Labels = baseLabels(c)

		var selector map[string]string
		var targetPort int
		if needsGateway {
			selector = gatewayLabels(c)
			targetPort = gatewayPort
		} else {
			selector = baseLabels(c)
			targetPort = neo4gPort
		}

		svc.Spec = corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: selector,
			Ports: []corev1.ServicePort{{
				Name:       "neo4g",
				Port:       neo4gPort,
				TargetPort: intstr.FromInt(targetPort),
			}},
		}
		return nil
	})
	return err
}

// --- Status ---

func (r *Neo4gClusterReconciler) reconcileStatus(ctx context.Context, c *neo4gv1alpha1.Neo4gCluster) error {
	sts := &appsv1.StatefulSet{}
	if err := r.Get(ctx, types.NamespacedName{Name: c.Name, Namespace: c.Namespace}, sts); err != nil {
		return err
	}

	c.Status.ReadyReplicas = sts.Status.ReadyReplicas
	c.Status.Endpoint = fmt.Sprintf("%s.%s.svc:%d", c.Name, c.Namespace, neo4gPort)

	switch {
	case sts.Status.ReadyReplicas == c.Spec.Replicas:
		c.Status.Phase = neo4gv1alpha1.PhaseRunning
		meta.SetStatusCondition(&c.Status.Conditions, metav1.Condition{
			Type:               "Available",
			Status:             metav1.ConditionTrue,
			Reason:             "AllReplicasReady",
			Message:            fmt.Sprintf("%d/%d replicas ready", sts.Status.ReadyReplicas, c.Spec.Replicas),
			LastTransitionTime: metav1.Now(),
		})
	case sts.Status.ReadyReplicas > 0:
		c.Status.Phase = neo4gv1alpha1.PhasePending
		meta.SetStatusCondition(&c.Status.Conditions, metav1.Condition{
			Type:               "Available",
			Status:             metav1.ConditionFalse,
			Reason:             "SomeReplicasNotReady",
			Message:            fmt.Sprintf("%d/%d replicas ready", sts.Status.ReadyReplicas, c.Spec.Replicas),
			LastTransitionTime: metav1.Now(),
		})
	default:
		c.Status.Phase = neo4gv1alpha1.PhasePending
		meta.SetStatusCondition(&c.Status.Conditions, metav1.Condition{
			Type:               "Available",
			Status:             metav1.ConditionFalse,
			Reason:             "NoReplicasReady",
			Message:            "waiting for pods to start",
			LastTransitionTime: metav1.Now(),
		})
	}

	return r.Status().Update(ctx, c)
}

// --- Helpers ---

func baseLabels(c *neo4gv1alpha1.Neo4gCluster) map[string]string {
	return map[string]string{
		labelCluster: c.Name,
		labelComp:    "db",
	}
}

func gatewayLabels(c *neo4gv1alpha1.Neo4gCluster) map[string]string {
	return map[string]string{
		labelCluster: c.Name,
		labelComp:    "gateway",
	}
}

func (r *Neo4gClusterReconciler) deleteIfExists(ctx context.Context, obj client.Object, key types.NamespacedName) error {
	if err := r.Get(ctx, key, obj); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}
	return r.Delete(ctx, obj)
}

func defaultStorageSize() resource.Quantity {
	return resource.MustParse("10Gi")
}

// SetupWithManager sets up the controller with the Manager.
func (r *Neo4gClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&neo4gv1alpha1.Neo4gCluster{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Named("neo4gcluster").
		Complete(r)
}
