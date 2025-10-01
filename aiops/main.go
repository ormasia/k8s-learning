package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type Evidence struct {
	Pod struct {
		Namespace         string                   `json:"namespace"`
		Name              string                   `json:"name"`
		Images            []map[string]string      `json:"images"`
		Phase             corev1.PodPhase          `json:"phase"`
		ContainerStatuses []corev1.ContainerStatus `json:"containerStatuses"`
	} `json:"pod"`
	Events []struct {
		Type, Reason, Message string `json:"type","reason","message"`
		Count                 int32  `json:"count"`
		LastTimestamp         string `json:"lastTimestamp,omitempty"`
	} `json:"events"`
	PreviousLogs string `json:"previousLogs"`
}

type LLMResult struct {
	Actions      []map[string]any `json:"actions"`
	Risks        []string         `json:"risks"`
	RollbackPlan []string         `json:"rollbackPlan"`
}

func main() {
	var (
		ns        = flag.String("n", "default", "namespace")
		name      = flag.String("p", "", "pod name")
		container = flag.String("c", "", "container name (optional)")
		ollama    = flag.String("ollama", "http://127.0.0.1:11434", "Ollama base URL")
		model     = flag.String("model", "qwen2.5:0.5b", "model name")
	)
	flag.Parse()
	if *name == "" {
		fmt.Fprintln(os.Stderr, "usage: aiops-propose -n <ns> -p <pod> [-c <container>] [--ollama URL] [--model name]")
		os.Exit(2)
	}

	ctx := context.Background()

	// 1) K8s client（集群内或 ~/.kube/config；两种都支持）
	cfg, cs := mustClientset()

	// 2) 采集证据：Pod / Events / previous logs
	ev, err := collect(ctx, cs, *ns, *name, *container)
	if err != nil {
		fmt.Fprintf(os.Stderr, "collect error: %v\n", err)
		os.Exit(1)
	}

	// 3) 调用 Ollama（结构化输出，强制 JSON）
	res, err := callOllama(ctx, *ollama, *model, ev)
	if err != nil {
		fmt.Fprintf(os.Stderr, "llm error: %v\n", err)
		os.Exit(1)
	}

	// 4) 输出建议（stdout 打印 JSON）
	out, _ := json.MarshalIndent(res, "", "  ")
	fmt.Println(string(out))

	_ = cfg // 防止未使用（如果你要继续扩展 SSA 执行，可用 cfg）
}

func mustClientset() (*rest.Config, *kubernetes.Clientset) {
	// 优先 in-cluster；否则走 kubeconfig（常见的双栈写法）
	if cfg, err := rest.InClusterConfig(); err == nil {
		cs, _ := kubernetes.NewForConfig(cfg)
		return cfg, cs
	}
	kubeconfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(),
		&clientcmd.ConfigOverrides{},
	)
	cfg, err := kubeconfig.ClientConfig()
	if err != nil {
		panic(err)
	}
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		panic(err)
	}
	return cfg, cs
}

func collect(ctx context.Context, cs *kubernetes.Clientset, ns, name, container string) (Evidence, error) {
	var ev Evidence

	// Pod 状态：包含 containerStatuses[*].state / reason（用来判断 ImagePullBackOff/CrashLoopBackOff 等）
	pod, err := cs.CoreV1().Pods(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return ev, err
	}
	ev.Pod.Namespace = pod.Namespace
	ev.Pod.Name = pod.Name
	ev.Pod.Phase = pod.Status.Phase
	ev.Pod.ContainerStatuses = pod.Status.ContainerStatuses
	for _, c := range pod.Spec.Containers {
		ev.Pod.Images = append(ev.Pod.Images, map[string]string{"name": c.Name, "image": c.Image})
	}

	// Events：按 involvedObject.kind/name 过滤，仅取该 Pod 的事件
	list, err := cs.CoreV1().Events(ns).List(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("involvedObject.kind=Pod,involvedObject.name=%s", name),
	})
	if err == nil {
		for _, e := range list.Items {
			ts := ""
			if !e.LastTimestamp.IsZero() {
				ts = e.LastTimestamp.Time.UTC().Format(time.RFC3339)
			}
			ev.Events = append(ev.Events, struct {
				Type, Reason, Message string `json:"type","reason","message"`
				Count                 int32  `json:"count"`
				LastTimestamp         string `json:"lastTimestamp,omitempty"`
			}{
				Type: e.Type, Reason: e.Reason, Message: e.Message, Count: e.Count, LastTimestamp: ts,
			})
		}
	}

	// previous 日志：等价 `kubectl logs -p`
	if container == "" && len(pod.Spec.Containers) > 0 {
		container = pod.Spec.Containers[0].Name
	}
	req := cs.CoreV1().Pods(ns).GetLogs(name, &corev1.PodLogOptions{
		Container: container, Previous: true,
	})
	if b, err := req.Do(ctx).Raw(); err == nil {
		ev.PreviousLogs = string(b)
	}

	return ev, nil
}

func callOllama(ctx context.Context, baseURL, model string, ev Evidence) (LLMResult, error) {
	var out struct {
		Message struct{ Content string } `json:"message"`
	}
	var res LLMResult

	sys := `你是Kubernetes SRE助手。输入是一个Pod的Evidence(JSON)。只输出严格JSON，结构为：
{
  "actions":[{"kind":"Patch","strategy":"ServerSideApply","objectRef":{"apiVersion":"apps/v1","kind":"Deployment","namespace":"...","name":"..."},"patch":{}}],
  "risks": ["..."],
  "rollbackPlan": ["..."]
}
仅允许的最小改动：镜像tag、imagePullSecrets、探针、资源配额。`

	payload := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": sys},
			{"role": "user", "content": mustJSON(ev)},
		},
		// 也可以传入完整 JSON Schema；先用 "json" 让模型输出 JSON
		"format": "json",
		"stream": false,
	}
	b, _ := json.Marshal(payload)
	req, _ := http.NewRequestWithContext(ctx, "POST", baseURL+"/api/chat", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return res, err
	}
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return res, err
	}
	if err := json.Unmarshal([]byte(out.Message.Content), &res); err != nil {
		return res, err
	}
	return res, nil
}

func mustJSON(v any) string { b, _ := json.Marshal(v); return string(b) }
