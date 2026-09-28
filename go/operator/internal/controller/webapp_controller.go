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

// Package controller 是 WebApp 的 reconciler（D20 练习）。
//
// 一个 WebApp 对应两个子资源：
//
//	WebApp ──owns──► Deployment（镜像、副本、端口、探针）
//	       ──owns──► Service（ClusterIP，port → targetPort）
//
// ⭐ 「owns」是通过 ownerReferences 表达的（controllerutil.SetControllerReference）：
//   - 删 WebApp → K8s 垃圾回收自动删两个子资源，你不用写删除逻辑
//   - 子资源被人改了/删了 → Owns() 让事件路由回这个 WebApp 的 reconcile → 你把它改回来
//
// D19 §2.3 的四条纪律在这里逐条兑现，代码里用 ⭐ 标了。
package controller

import (
	"context"
	"fmt"
	"maps"
	"slices"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	appsv1alpha1 "github.com/hzjconan/learn-program-language/go/operator/api/v1alpha1"

	"k8s.io/apimachinery/pkg/api/meta"
)

// WebAppReconciler reconciles a WebApp object
type WebAppReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// RBAC marker：controller-gen 从这些注释生成 config/rbac/role.yaml。
// ⚠️ 你的 controller 要碰 Deployment 和 Service，就必须在这里声明 ——
// 本机 make run 用的是你的 kubeconfig（admin），什么都能做；
// 部署进集群后用的是 ServiceAccount，少了 marker 就是 403 Forbidden。
// 这是「本机能跑、集群里不行」的头号原因（§5.3）。
// +kubebuilder:rbac:groups=apps.golearn.dev,resources=webapps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.golearn.dev,resources=webapps/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps.golearn.dev,resources=webapps/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete

// Reconcile 把一个 WebApp 的现状调到和 spec 一致。
//
// ⭐ 它拿到的只有 req.NamespacedName —— 一个 key。不知道「发生了什么」，
// 也不该知道（D19 §2.3 ②：只看现状，不信事件）。每次都从头看一遍。
//
// TODO(D20) ②：按下面的步骤实现。
//
//	① 取 WebApp：r.Get(ctx, req.NamespacedName, &app)
//	   NotFound → return ctrl.Result{}, nil（被删了，子资源由 GC 处理，什么都不用做）
//	   其它错误 → return ctrl.Result{}, err（controller-runtime 会限速重试 —— D19 §5.3 的 AddRateLimited）
//
//	② 确保 Deployment：
//	   deploy := &appsv1.Deployment{ObjectMeta: {Name: app.Name, Namespace: app.Namespace}}   ← 只填 key 的空壳
//	   controllerutil.CreateOrUpdate(ctx, r.Client, deploy, func() error {
//	       desired := desiredDeployment(&app)
//	       ...把 desired 的字段抄到 deploy 上（§4.2 的 A 或 B 两档）...
//	       return controllerutil.SetControllerReference(&app, deploy, r.Scheme)   ← ⭐ 在 mutateFn 里、对 deploy 调
//	   })
//	   ⚠️ desired 每次必须算出【完全一样】的结果（Env 排序、没有 time.Now()），
//	      否则内容真的变了 → generation +1 → Pod 滚动 → Owns 事件 → 再来（§4.2）。
//
//	③ 确保 Service：同上，desiredService 已经写好，照着做。
//
//	④ 更新 status：
//	   - ObservedGeneration = app.Generation
//	   - ReadyReplicas 从 Deployment.Status.ReadyReplicas 抄
//	   - meta.SetStatusCondition(&app.Status.Conditions, metav1.Condition{Type: Available, ...})
//	   - r.Status().Update(ctx, &app)   ← ⭐ 是 Status()，不是 Update()（§4.3）
//
//	⑤ return ctrl.Result{}, nil
//	   ⭐ 不需要 RequeueAfter：Deployment 的变化会通过 Owns() 触发下一次 reconcile（§3.4）。
//	   D19 §2.3 ③「每次只做一步然后 return」在这里的形态就是：不等 Pod Ready，return，等事件。
//
// ⚠️ 幂等自检（D19 §2.3 ①）：同一个 spec 连跑两次 Reconcile，第二次不该有任何写操作。
// 测试里有一条专门验这个（TestReconcile_IsIdempotent）。
func (r *WebAppReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// TODO(D20) ②
	var app appsv1alpha1.WebApp
	if err := r.Get(ctx, req.NamespacedName, &app); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err) // NotFound → nil；其它 → err 重试
	}

	desiredD := desiredDeployment(&app)

	// deployment
	// 只填 name/namespace —— 这是只填了 key 的空壳，不是内容， 它把集群里的现状 Get 进这个壳，再把壳交给mutateFn改
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: app.Name, Namespace: app.Namespace},
	}
	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, deploy, func() error {
		deploy.Spec.Replicas = desiredD.Spec.Replicas
		if deploy.CreationTimestamp.IsZero() { // ⚠️ Selector 只在创建时设，之后不可变
			deploy.Spec.Selector = desiredD.Spec.Selector
		}
		deploy.Spec.Template = desiredD.Spec.Template
		return controllerutil.SetControllerReference(&app, deploy, r.Scheme)
	})
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("ensure deployment: %w", err)
	}
	log.Info("deployment reconciled", "op", op)

	// service
	desiredS := desiredService(&app)
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: app.Name, Namespace: app.Namespace},
	}
	op, err = controllerutil.CreateOrUpdate(ctx, r.Client, service, func() error {
		service.Spec.Type = desiredS.Spec.Type
		service.Spec.Selector = desiredS.Spec.Selector
		service.Spec.Ports = desiredS.Spec.Ports
		return controllerutil.SetControllerReference(&app, service, r.Scheme)
	})
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("ensure service: %w", err)
	}
	log.Info("service reconciled", "op", op)

	// 更新状态
	app.Status.ObservedGeneration = app.Generation
	app.Status.ReadyReplicas = deploy.Status.ReadyReplicas

	want := int32(1)
	if app.Spec.Replicas != nil {
		want = *app.Spec.Replicas
	}
	ready := deploy.Status.ReadyReplicas

	cond := metav1.Condition{Type: appsv1alpha1.ConditionAvailable}
	switch ready {
	case want:
		cond.Status, cond.Reason = metav1.ConditionTrue, "ReplicasReady"
		cond.Message = fmt.Sprintf("%d/%d replicas ready", ready, want)
	default:
		cond.Status, cond.Reason = metav1.ConditionFalse, "ReplicasNotReady"
		cond.Message = fmt.Sprintf("%d/%d replicas ready", ready, want)
	}
	meta.SetStatusCondition(&app.Status.Conditions, cond)
	err = r.Status().Update(ctx, &app)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("update status: %w", err)
	}

	log.Info("status updated")

	return ctrl.Result{}, nil

}

// desiredDeployment 根据 spec 算出「应该长什么样」的 Deployment。
//
// ⭐ 纯函数：输入 WebApp，输出对象，不碰集群。这样它能用普通 go test 表驱动测，
// 不需要 envtest（D14 §6 的分层测试又来了：纯逻辑一层、和集群交互一层）。
//
// TODO(D20) ③：补完 PodSpec —— 容器名 "app"、镜像、端口、readinessProbe(GET healthPath:port)、
// Env（等你加完 ① 之后）。labels 用 labelsFor(app)，selector 和 template labels 必须一致。
func desiredDeployment(app *appsv1alpha1.WebApp) *appsv1.Deployment {
	labels := labelsFor(app)
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      app.Name,
			Namespace: app.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: app.Spec.Replicas,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					// TODO(D20) ③
					Containers: []corev1.Container{{
						Name:  "app",
						Image: app.Spec.Image,
						Ports: []corev1.ContainerPort{{
							Name:          "http",
							ContainerPort: app.Spec.Port,
							Protocol:      corev1.ProtocolTCP,
						}},
						Env: envVars(app.Spec.Env),
						ReadinessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: app.Spec.HealthPath, Port: intstr.FromInt32(app.Spec.Port)}},
						},
					}},
				},
			},
		},
	}
}

// desiredService 已经写好，是 ② 的参照。
func desiredService(app *appsv1alpha1.WebApp) *corev1.Service {
	labels := labelsFor(app)
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      app.Name,
			Namespace: app.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: labels,
			Ports: []corev1.ServicePort{{
				Name:       "http",
				Port:       app.Spec.Port,
				TargetPort: intstr.FromInt32(app.Spec.Port),
				Protocol:   corev1.ProtocolTCP,
			}},
		},
	}
}

// labelsFor 是子资源的 label，也是 selector。
// ⚠️ 只放不会变的东西（D19 §3.4：selector 创建后不可变）—— 别把 image 或 generation 放进来。
func labelsFor(app *appsv1alpha1.WebApp) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       app.Name,
		"app.kubernetes.io/managed-by": "golearn-operator",
	}
}

// SetupWithManager 把 reconciler 注册到 Manager。
//
// TODO(D20) ④：加上
//
//	Owns(&appsv1.Deployment{}).
//	Owns(&corev1.Service{}).
//
// ⭐ 这两行就是 D19 §4.4 那张图的「informer → 队列」接线：
// Manager 会为 Deployment / Service 各起一个 informer（共享缓存），
// 它们变化时，controller-runtime 顺着 ownerReference 找到父 WebApp，把【父的 key】入队。
// 没有这两行：有人 kubectl delete 了你的 Deployment，你永远不知道。
func (r *WebAppReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&appsv1alpha1.WebApp{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Named("webapp").
		Complete(r)
}

func envVars(env map[string]string) []corev1.EnvVar {
	keys := slices.Sorted(maps.Keys(env))
	result := make([]corev1.EnvVar, 0, len(keys))
	for _, k := range keys {
		result = append(result, corev1.EnvVar{
			Name:  k,
			Value: env[k],
		})
	}
	return result
}
