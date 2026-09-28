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

// 本文件用 TestMain 起一个 envtest 环境，整包共用（D20 §6）。
//
// envtest = 【真的】kube-apiserver + etcd 二进制（make setup-envtest 下到 bin/k8s/），
// 没有 kubelet / scheduler / controller-manager。所以：
//
//	✅ 能测：CRD 校验、默认值、你的 Reconcile 对 API 对象做了什么、ownerReference、status 子资源
//	❌ 不能测：Pod 真的起来、Deployment 的 ReadyReplicas 会自己变（没有 controller-manager）
//
// 这和 D15 §2 一样是「这层测试看不见什么」的问题 —— envtest 里 Deployment 永远不会 Ready，
// 所以 status.readyReplicas 的测试要自己把 Deployment.Status 改了再触发 reconcile。
//
// ⭐ 结构和 D15 的 TestMain + testcontainers 一模一样：起环境 → m.Run() → 清理 → os.Exit。
// kubebuilder 默认生成的是 Ginkgo（BDD 风格）套件，这里换成标准 testing —— 少学一个框架，
// 而且 D15 的所有习惯（t.Cleanup、表驱动、精确断言）直接沿用。

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"testing"

	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	appsv1alpha1 "github.com/hzjconan/learn-program-language/go/operator/api/v1alpha1"
)

var (
	cfg       *rest.Config
	k8sClient client.Client
)

func TestMain(m *testing.M) {
	logf.SetLogger(zap.New(zap.UseDevMode(true), zap.WriteTo(os.Stderr)))

	if err := appsv1alpha1.AddToScheme(scheme.Scheme); err != nil {
		log.Fatalf("注册 scheme: %v", err)
	}

	testEnv := &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true, // ⭐ CRD 目录不存在就失败，不 Skip（D15 §3.1）
	}
	if dir := firstEnvtestBinaryDir(); dir != "" {
		testEnv.BinaryAssetsDirectory = dir
	}

	var err error
	cfg, err = testEnv.Start()
	if err != nil {
		log.Fatalf("起 envtest 失败（先跑 make setup-envtest）: %v", err)
	}

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		stopEnv(testEnv)
		log.Fatalf("建 client: %v", err)
	}

	code := m.Run()
	stopEnv(testEnv) // os.Exit 不跑 defer —— D15 / MISTAKES #28
	os.Exit(code)
}

func stopEnv(env *envtest.Environment) {
	if err := env.Stop(); err != nil {
		log.Printf("停 envtest: %v", err)
	}
}

// firstEnvtestBinaryDir 找 make setup-envtest 下载的二进制目录（bin/k8s/<version>-<os>-<arch>/）。
func firstEnvtestBinaryDir() string {
	base := filepath.Join("..", "..", "bin", "k8s")
	entries, err := os.ReadDir(base)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.IsDir() {
			return filepath.Join(base, e.Name())
		}
	}
	return ""
}

// testCtx 给每个测试一个带取消的 ctx。
func testCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return ctx
}
