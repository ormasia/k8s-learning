/*
Copyright 2025.

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

package main

import (
	"crypto/tls"
	"flag"
	"os"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	aiopsv1alpha1 "github.com/ormasia/aiops-operator/api/v1alpha1"
	"github.com/ormasia/aiops-operator/internal/controller"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(aiopsv1alpha1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}
func main() {
	// 1. 初始化日志系统（重要！）
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
	// 读取环境变量/flags（示例：给执行器传 Ollama 配置）
	ollamaURL := getenv("OLLAMA_URL", "http://127.0.0.1:11434")
	ollamaModel := getenv("OLLAMA_MODEL", "qwen2.5:7b")

	// ... 解析 flags、初始化日志略
	var metricsAddr string
	var probeAddr string
	var enableLeaderElection bool
	var secureMetrics bool
	var tlsOpts []func(*tls.Config)

	webhookServer := webhook.NewServer(webhook.Options{TLSOpts: tlsOpts})
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsserver.Options{BindAddress: metricsAddr, SecureServing: secureMetrics, TLSOpts: tlsOpts},
		WebhookServer:          webhookServer,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "85bbf4a4.example.com",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// 注册 执行/推理器（RemediationExecutorReconciler）
	if err = (&controller.RemediationExecutorReconciler{
		Client:      mgr.GetClient(),
		Scheme:      mgr.GetScheme(),
		OllamaURL:   ollamaURL,   // <<< 传入自定义依赖
		OllamaModel: ollamaModel, // <<<
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Remediation")
		os.Exit(1)
	}

	// 注册 监控器（PodDetectorReconciler）
	if err = (&controller.PodDetectorReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Pod")
		os.Exit(1)
	}

	// 探针
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil { /* ... */
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil { /* ... */
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
