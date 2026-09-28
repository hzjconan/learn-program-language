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
	"fmt"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appsv1alpha1 "github.com/hzjconan/learn-program-language/go/operator/api/v1alpha1"
)

var defaultNS = "default"

var healthPath = "/healthz"

// ⭐ 测试策略：【直接调 Reconcile】，不起 Manager。
//
// 起 Manager 的话 reconcile 在后台 goroutine 里跑，测试得轮询等结果（Eventually），
// 慢、不确定、失败时不知道是逻辑错还是没等够。直接调的话：
//
//	建 WebApp → r.Reconcile(ctx, req) → 查子资源
//
// 同步、确定、一次一步。Manager 的接线（Owns / 事件路由）是 controller-runtime 的事，
// 那部分留给 §7 ③ 真部署进 kind 去验。
// 这和 D14 §6「handler 层直接调 handler，不起 http.Server」是同一个取舍。

func newReconciler() *WebAppReconciler {
	return &WebAppReconciler{Client: k8sClient, Scheme: scheme.Scheme}
}

// newWebApp 建一个 WebApp 进 envtest，名字从测试名派生，测试结束自动删。
func newWebApp(t *testing.T) *appsv1alpha1.WebApp {
	t.Helper()
	app := &appsv1alpha1.WebApp{
		ObjectMeta: metav1.ObjectMeta{
			Name:      nameFor(t),
			Namespace: defaultNS,
		},
		Spec: appsv1alpha1.WebAppSpec{
			Image:      "golearn-api:test",
			Replicas:   ptr.To[int32](2),
			Port:       8080,
			HealthPath: healthPath,
		},
	}

	ctx := testCtx(t)
	if err := k8sClient.Create(ctx, app); err != nil {
		t.Fatalf("建 WebApp: %v", err)
	}
	t.Cleanup(func() {
		_ = k8sClient.Delete(ctx, app)
		// ⚠️ envtest 里没有 GC controller，ownerReference 不会真的级联删除 —— 子资源要自己清
		_ = k8sClient.Delete(ctx, &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: app.Name, Namespace: app.Namespace}})
		_ = k8sClient.Delete(ctx, &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: app.Name, Namespace: app.Namespace}})
	})
	return app
}

// nameFor 把测试名变成合法的 K8s 名字（小写、无下划线/斜杠）。
func nameFor(t *testing.T) string {
	n := strings.ToLower(t.Name())
	n = strings.NewReplacer("/", "-", "_", "-", " ", "-").Replace(n)
	return fmt.Sprintf("wa-%.40s", n)
}

func reconcile(t *testing.T, app *appsv1alpha1.WebApp) ctrl.Result {
	t.Helper()
	res, err := newReconciler().Reconcile(testCtx(t), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: app.Name, Namespace: app.Namespace},
	})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	return res
}

func getDeployment(t *testing.T, app *appsv1alpha1.WebApp) *appsv1.Deployment {
	t.Helper()
	var d appsv1.Deployment
	if err := k8sClient.Get(testCtx(t), client.ObjectKeyFromObject(app), &d); err != nil {
		t.Fatalf("取 Deployment: %v", err)
	}
	return &d
}

// ---------- ① 建出来 ----------

func TestReconcile_CreatesDeploymentAndService(t *testing.T) {
	app := newWebApp(t)
	reconcile(t, app)

	d := getDeployment(t, app)
	if got := *d.Spec.Replicas; got != 2 {
		t.Errorf("Deployment replicas = %d, want 2", got)
	}
	if len(d.Spec.Template.Spec.Containers) != 1 || d.Spec.Template.Spec.Containers[0].Image != "golearn-api:test" {
		t.Errorf("容器 = %+v, want 一个 image=golearn-api:test 的容器", d.Spec.Template.Spec.Containers)
	}

	// ⭐ ownerReference 是 GC 和 Owns() 事件路由的基础，必须有，而且 controller=true
	owner := metav1.GetControllerOf(d)
	if owner == nil || owner.Kind != "WebApp" || owner.Name != app.Name {
		t.Errorf("Deployment 的 controller ownerReference = %+v, want WebApp/%s\n"+
			"（用 controllerutil.SetControllerReference）", owner, app.Name)
	}

	var svc corev1.Service
	if err := k8sClient.Get(testCtx(t), client.ObjectKeyFromObject(app), &svc); err != nil {
		t.Fatalf("取 Service: %v", err)
	}
	if len(svc.Spec.Ports) != 1 || svc.Spec.Ports[0].Port != 8080 {
		t.Errorf("Service ports = %+v, want [8080]", svc.Spec.Ports)
	}
	if metav1.GetControllerOf(&svc) == nil {
		t.Error("Service 没有 controller ownerReference")
	}
}

// ---------- ② 幂等 ----------

// TestReconcile_IsIdempotent 是今天最重要的一条（D19 §2.3 ①）。
//
// 同一个 spec 跑两次，第二次【不该有任何写】。判据是 resourceVersion：
// 只要对象被 Update 过，哪怕内容一样，resourceVersion 也会变（D19 §3.3）。
//
// ⚠️ 它抓的是「desired 不确定」：Env 用 map 直接 range 顺序随机、用了 time.Now()……
// 内容真的变了 → 服务端真的写 → resourceVersion 变。
// 它抓【不到】「整个覆盖 Spec」—— 那种 PUT 服务端会 no-op（讲义 §4.2 实测），
// 只是 CreateOrUpdate 每次报 updated。要抓那个得断言 OperationResult == None。
func TestReconcile_IsIdempotent(t *testing.T) {
	app := newWebApp(t)
	reconcile(t, app)
	rv1 := getDeployment(t, app).ResourceVersion

	reconcile(t, app)
	rv2 := getDeployment(t, app).ResourceVersion

	if rv1 != rv2 {
		t.Errorf("第二次 Reconcile 改了 Deployment（resourceVersion %s → %s）\n"+
			"（⚠️ desired 是不是每次算出来不一样？Env 排序了吗、有没有 time.Now()）", rv1, rv2)
	}
}

// ---------- 下面的 TODO(D20) ⑤ 自己写 ----------

// TestReconcile_NotFoundIsNoop
//   对一个不存在的 name 调 Reconcile → 返回 nil error、空 Result。
//   ⚠️ 返回 err 的话 controller-runtime 会一直重试一个永远不会出现的对象。

// TestReconcile_RestoresDrift
//   建 → reconcile → 手动把 Deployment 的 replicas 改成 5（模拟有人 kubectl scale）
//   → 再 reconcile → replicas 回到 2。
//   ⭐ 这就是 level-triggered：不管谁改的、为什么改，看到不一致就调回去。

// TestReconcile_UpdatesOnSpecChange
//   建 → reconcile → 改 WebApp.Spec.Image 为 "golearn-api:v2"（k8sClient.Update）
//   → reconcile → Deployment 容器镜像变成 v2。

// TestReconcile_SetsStatus
//   建 → reconcile → 手动把 Deployment.Status.ReadyReplicas 设成 2
//   （k8sClient.Status().Update —— envtest 没有 controller-manager，没人替你填）
//   → reconcile → WebApp.Status.ReadyReplicas == 2，
//   Available condition 为 True，ObservedGeneration == app.Generation。
//   反过来 ReadyReplicas=1 时 Available 应该是 False。

// TestDesiredDeployment（纯函数，不用 envtest，表驱动）
//   - healthPath / port 进了 readinessProbe
//   - Env map 变成了容器 env（排序要稳定！map 无序，直接 range 会让 ② 幂等测试随机红 —— D3）
//   - selector 和 template labels 一致

func TestReconcile_NotFoundIsNoop(t *testing.T) {
	res, err := newReconciler().Reconcile(testCtx(t), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "nonexistent", Namespace: defaultNS},
	})
	if err != nil {
		t.Errorf("Reconcile error: %v", err)
	}
	if res != (ctrl.Result{}) {
		t.Errorf("Reconcile result = %v, want empty", res)
	}
}

func TestReconcile_RestoresDrift(t *testing.T) {
	app := newWebApp(t)
	reconcile(t, app)

	// 手动把 Deployment 改成 replicas=5（模拟 drift）
	d := getDeployment(t, app)
	d.Spec.Replicas = ptr.To[int32](5)
	if err := k8sClient.Update(testCtx(t), d); err != nil {
		t.Fatalf("改 Deployment: %v", err)
	}

	reconcile(t, app)

	// 应该被拉回 spec 的值 2
	d2 := getDeployment(t, app)
	got := d2.Spec.Replicas
	if *got != 2 {
		t.Errorf("漂移修复后 replicas = %d, want 2\n（⚠️ Reconcile 看到 spec 和现状不一致，应该把 replicas 改回去）", *got)
	}
}

func TestReconcile_UpdatesOnSpecChange(t *testing.T) {
	// 建 → reconcile → 改 WebApp.Spec.Image 为 "golearn-api:v2"（k8sClient.Update）
	// → reconcile → Deployment 容器镜像变成 v2。
	app := newWebApp(t)
	reconcile(t, app)

	// 改 spec（模拟用户 kubectl edit webapp）
	// ⚠️ reconcile 会更新 status → resourceVersion 变了，必须重新 Get 再 Update
	ctx := testCtx(t)
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(app), app); err != nil {
		t.Fatalf("重新 Get WebApp: %v", err)
	}
	app.Spec.Image = "golearn-api:v2"
	if err := k8sClient.Update(ctx, app); err != nil {
		t.Fatalf("改 WebApp.Spec: %v", err)
	}

	reconcile(t, app)

	// 子资源应该跟着 spec 变
	got := getDeployment(t, app).Spec.Template.Spec.Containers[0].Image
	if got != "golearn-api:v2" {
		t.Errorf("容器镜像 = %q, want golearn-api:v2\n（⚠️ spec 变了，Reconcile 应该同步到 Deployment）", got)
	}
}

func TestReconcile_SetsStatus(t *testing.T) {
	app := newWebApp(t)
	ctx := testCtx(t)

	// 第一次 reconcile：Deployment 刚建，ReadyReplicas 还是 0
	reconcile(t, app)

	// 手动把 Deployment.Status 填上
	d := getDeployment(t, app)
	d.Status.Replicas = 2
	d.Status.ReadyReplicas = 2
	if err := k8sClient.Status().Update(ctx, d); err != nil {
		t.Fatalf("改 Deployment status: %v", err)
	}

	reconcile(t, app)

	// 断言 WebApp.Status 被正确更新
	var got appsv1alpha1.WebApp
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(app), &got); err != nil {
		t.Fatalf("取 WebApp: %v", err)
	}

	if got.Status.ReadyReplicas != 2 {
		t.Errorf("ReadyReplicas = %d, want 2", got.Status.ReadyReplicas)
	}
	if got.Status.ObservedGeneration != got.Generation {
		t.Errorf("ObservedGeneration = %d, want %d", got.Status.ObservedGeneration, got.Generation)
	}

	cond := meta.FindStatusCondition(got.Status.Conditions, appsv1alpha1.ConditionAvailable)
	if cond == nil {
		t.Fatal("Available condition 没设")
	}
	if cond.Status != metav1.ConditionTrue {
		t.Errorf("Available condition = %s, want True（ReadyReplicas 匹配 spec）", cond.Status)
	}

	// 反过来：ReadyReplicas 不匹配 → Available=False
	d.Status.Replicas = 2
	d.Status.ReadyReplicas = 1
	if err := k8sClient.Status().Update(ctx, d); err != nil {
		t.Fatalf("改 Deployment status: %v", err)
	}
	reconcile(t, app)

	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(app), &got); err != nil {
		t.Fatalf("取 WebApp: %v", err)
	}
	cond = meta.FindStatusCondition(got.Status.Conditions, appsv1alpha1.ConditionAvailable)
	if cond == nil {
		t.Fatal("Available condition 没设")
	}
	if cond.Status != metav1.ConditionFalse {
		t.Errorf("Available condition = %s, want False（ReadyReplicas=1 ≠ spec=2）", cond.Status)
	}
}

func TestDesiredDeployment(t *testing.T) {
	tests := []struct {
		name     string
		env      map[string]string
		health   string
		port     int32
		wantEnv  []struct{ Name, Value string }
		wantPath string
	}{
		{
			name:   "Env 按 key 排序输出",
			env:    map[string]string{"Z_KEY": "z", "A_KEY": "a", "M_KEY": "m"},
			health: "/readyz",
			port:   8080,
			wantEnv: []struct{ Name, Value string }{
				{"A_KEY", "a"}, {"M_KEY", "m"}, {"Z_KEY", "z"},
			},
			wantPath: "/readyz",
		},
		{
			name:     "Env 为空 → 没有 Env",
			env:      map[string]string{},
			health:   healthPath,
			port:     8080,
			wantEnv:  []struct{ Name, Value string }{},
			wantPath: healthPath,
		},
		{
			name:     "healthPath 和 port",
			env:      nil,
			health:   "/",
			port:     3000,
			wantEnv:  []struct{ Name, Value string }{},
			wantPath: "/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := &appsv1alpha1.WebApp{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: defaultNS},
				Spec: appsv1alpha1.WebAppSpec{
					Image:      "nginx:latest",
					Replicas:   ptr.To[int32](1),
					Port:       tt.port,
					HealthPath: tt.health,
					Env:        tt.env,
				},
			}

			d := desiredDeployment(app)

			// readinessProbe
			c := d.Spec.Template.Spec.Containers[0]
			if c.ReadinessProbe == nil || c.ReadinessProbe.HTTPGet == nil {
				t.Fatal("readinessProbe 没设")
			}
			if c.ReadinessProbe.HTTPGet.Path != tt.wantPath {
				t.Errorf("readinessProbe path = %q, want %q",
					c.ReadinessProbe.HTTPGet.Path, tt.wantPath)
			}
			if gotPort := c.ReadinessProbe.HTTPGet.Port.IntValue(); int32(gotPort) != tt.port {
				t.Errorf("readinessProbe port = %d, want %d", gotPort, tt.port)
			}

			// Env —— 稳定排序
			if len(c.Env) != len(tt.wantEnv) {
				t.Fatalf("Env len = %d, want %d", len(c.Env), len(tt.wantEnv))
			}
			for i, e := range c.Env {
				if e.Name != tt.wantEnv[i].Name || e.Value != tt.wantEnv[i].Value {
					t.Errorf("Env[%d] = %s=%q, want %s=%q",
						i, e.Name, e.Value, tt.wantEnv[i].Name, tt.wantEnv[i].Value)
				}
			}

			// selector 和 template labels 一致
			selector := d.Spec.Selector.MatchLabels
			templateLabels := d.Spec.Template.Labels
			if len(selector) != len(templateLabels) {
				t.Fatalf("selector labels %v != template labels %v", selector, templateLabels)
			}
			for k, v := range selector {
				if templateLabels[k] != v {
					t.Errorf("selector[%q]=%q, template[%q]=%q", k, v, k, templateLabels[k])
				}
			}
		})
	}
}
