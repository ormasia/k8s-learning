# 📚 Informer 和 WorkQueue 在 aiops-operator 中的体现

## 🎯 核心答案

**你的代码中没有直接看到 Informer 和 WorkQueue 的实现代码**，因为：

✅ **controller-runtime 框架已经帮你封装好了！**

你只需要调用 `ctrl.NewControllerManagedBy(mgr).For(&corev1.Pod{}).Complete(r)`，框架会自动创建：
1. **Informer** - 监听 API Server 资源变化
2. **WorkQueue** - 排队处理事件
3. **Event Handler** - 将事件转换为 Reconcile 请求

---

## 🔍 一、Informer 和 WorkQueue 在哪里？

### 1.1 隐藏在 `SetupWithManager()` 中

**你的代码：**
```go
// internal/controller/pod_controller.go (第 163-167 行)
func (r *PodDetectorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Pod{}).        // ← 这里会自动创建 Pod Informer
		Complete(r)                 // ← 这里会创建 WorkQueue 并启动 worker
}
```

**等价于底层 client-go 代码：**
```go
// controller-runtime 内部实现（简化版）
func (blder *Builder) For(object client.Object) *Builder {
	// 1. 创建 Informer
	informer, err := mgr.GetCache().GetInformerForKind(ctx, gvk)
	
	// 2. 创建 WorkQueue
	workqueue := workqueue.NewRateLimitingQueue(
		workqueue.DefaultControllerRateLimiter(),
	)
	
	// 3. 注册 EventHandler
	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			// 将 Add 事件转换为 Reconcile Request
			workqueue.Add(reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      obj.GetName(),
					Namespace: obj.GetNamespace(),
				},
			})
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			// 将 Update 事件转换为 Reconcile Request
			workqueue.Add(...)
		},
		DeleteFunc: func(obj interface{}) {
			// 将 Delete 事件转换为 Reconcile Request
			workqueue.Add(...)
		},
	})
	
	return blder
}

func (blder *Builder) Complete(r reconcile.Reconciler) error {
	// 4. 启动 worker goroutines
	for i := 0; i < maxConcurrentReconciles; i++ {
		go func() {
			for {
				// 从 WorkQueue 取出 Reconcile Request
				req, shutdown := workqueue.Get()
				if shutdown {
					return
				}
				
				// 调用你的 Reconcile() 方法
				result, err := r.Reconcile(ctx, req)
				
				// 根据结果决定是否重新排队
				if err != nil || result.Requeue {
					workqueue.AddRateLimited(req)
				} else {
					workqueue.Forget(req)
				}
				workqueue.Done(req)
			}
		}()
	}
	
	// 5. 启动 Informer
	go informer.Run(stopCh)
	
	return nil
}
```

---

## 🏗️ 二、完整架构图

```
┌─────────────────────────────────────────────────────────────────────┐
│                          Kubernetes API Server                       │
│                      (Pod, Remediation Resources)                    │
└───────────────────────┬─────────────────────────────────────────────┘
                        │ Watch
                        │ (HTTP Long Polling)
                        ▼
┌─────────────────────────────────────────────────────────────────────┐
│              controller-runtime Manager (mgr)                        │
│  ┌────────────────────────────────────────────────────────────────┐ │
│  │                    Shared Informer Cache                        │ │
│  │  ┌──────────────────┐     ┌──────────────────┐                 │ │
│  │  │ Pod Informer     │     │ Remediation      │  ← Informer     │ │
│  │  │ - Indexer        │     │ Informer         │    (自动创建)    │ │
│  │  │ - DeltaFIFO      │     │ - Indexer        │                 │ │
│  │  └────────┬─────────┘     └────────┬─────────┘                 │ │
│  └───────────┼──────────────────────────┼──────────────────────────┘ │
│              │                          │                            │
│              │ EventHandler             │ EventHandler               │
│              │ (Add/Update/Delete)      │ (Add/Update/Delete)        │
│              ▼                          ▼                            │
│  ┌─────────────────────────┐  ┌─────────────────────────┐          │
│  │ Pod WorkQueue           │  │ Remediation WorkQueue   │ ← Queue  │
│  │ (RateLimitingQueue)     │  │ (RateLimitingQueue)     │   (自动) │
│  └────────┬────────────────┘  └────────┬────────────────┘          │
└───────────┼───────────────────────────┼─────────────────────────────┘
            │                           │
            │ Worker Goroutines (1-N)   │ Worker Goroutines (1-N)
            ▼                           ▼
┌───────────────────────────┐  ┌───────────────────────────┐
│ PodDetectorReconciler     │  │ RemediationExecutor       │
│ - Reconcile(ctx, req)     │  │ Reconciler                │ ← 你的代码
│   ↓                       │  │ - Reconcile(ctx, req)     │
│   1. Get Pod              │  │   ↓                       │
│   2. IsAnomalous?         │  │   1. Get Remediation      │
│   3. Create Remediation   │  │   2. Check Approved?      │
│   4. Update Status        │  │   3. Call LLM             │
└───────────────────────────┘  │   4. Apply Patch (SSA)    │
                               └───────────────────────────┘
```

---

## 🔑 三、关键代码位置

### 3.1 Informer 的隐式创建

**位置：** `internal/controller/pod_controller.go:163-167`

```go
func (r *PodDetectorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Pod{}).        // ← 自动创建 Pod Informer
		Complete(r)
}
```

**框架行为：**
- `For(&corev1.Pod{})` → 创建 **Pod Informer**
- Informer 会 **Watch** API Server 的 `/api/v1/pods` 资源
- 当 Pod 发生变化（Add/Update/Delete），触发 EventHandler

---

### 3.2 WorkQueue 的隐式创建

**位置：** 同上（`Complete(r)` 内部）

```go
// controller-runtime 内部逻辑（简化）
workqueue := workqueue.NewRateLimitingQueue(
	workqueue.DefaultControllerRateLimiter(),
)
```

**WorkQueue 配置：**
- **类型：** `RateLimitingQueue`（带限流功能）
- **限流策略：** 指数退避（失败后延迟：1s → 2s → 4s → ... → 最大 1000s）
- **并发 Worker：** 默认 1 个（可通过 `MaxConcurrentReconciles` 调整）

---

### 3.3 EventHandler 的隐式注册

**框架行为：**
```go
// controller-runtime 自动注册
informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
	AddFunc: func(obj interface{}) {
		// 1. 从 Informer 本地缓存获取对象
		pod := obj.(*corev1.Pod)
		
		// 2. 转换为 Reconcile Request
		req := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      pod.Name,
				Namespace: pod.Namespace,
			},
		}
		
		// 3. 放入 WorkQueue
		workqueue.Add(req)
	},
	UpdateFunc: func(oldObj, newObj interface{}) {
		// 仅当对象发生实际变化时才入队
		if oldObj.(*corev1.Pod).ResourceVersion != newObj.(*corev1.Pod).ResourceVersion {
			workqueue.Add(...)
		}
	},
	DeleteFunc: func(obj interface{}) {
		workqueue.Add(...)
	},
})
```

---

### 3.4 你的 Reconcile() 方法何时被调用？

**触发流程：**
```
1. API Server 发送 Watch Event (Pod 变化)
   ↓
2. Informer 接收事件并更新本地缓存
   ↓
3. EventHandler 将事件转换为 Reconcile Request
   ↓
4. WorkQueue 排队 (支持去重、限流)
   ↓
5. Worker 从队列取出 Request
   ↓
6. 调用你的 Reconcile(ctx, req) 方法
   ↓
7. 根据返回值决定是否重新入队：
   - err != nil → 重新入队（带指数退避）
   - result.Requeue = true → 立即重新入队
   - result.RequeueAfter = 5*time.Second → 5秒后重新入队
   - 正常返回 → 从队列移除
```

**你的代码示例：**
```go
// internal/controller/pod_controller.go:87-161
func (r *PodDetectorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx).WithValues("pod", req.NamespacedName)
	log.Info("Reconcile triggered")  // ← Worker 从 WorkQueue 取出后调用
	
	// 1. 从 Informer 缓存获取 Pod
	pod := &corev1.Pod{}
	if err := r.Get(ctx, req.NamespacedName, pod); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	
	// 2. 检查是否异常
	if !evidence.IsAnomalous(pod) {
		return ctrl.Result{}, nil  // ← 正常返回，从队列移除
	}
	
	// 3. 创建 Remediation
	rem := &v1alpha1.Remediation{...}
	if err := r.Create(ctx, rem); err != nil {
		return ctrl.Result{}, err  // ← 返回错误，重新入队（带退避）
	}
	
	return ctrl.Result{}, nil  // ← 成功，从队列移除
}
```

---

## 🔬 四、如何验证 Informer 和 WorkQueue 的存在？

### 4.1 查看 Informer 的日志

```bash
# 启动 operator 时会看到 Informer 启动日志
kubectl logs -n aiops-operator-system deploy/aiops-operator-controller-manager

# 输出示例：
# 2025-10-02T10:30:15.123Z	INFO	Starting EventSource	{"controller": "pod", "source": "kind source: *v1.Pod"}
# 2025-10-02T10:30:15.234Z	INFO	Starting Controller	{"controller": "pod"}
# 2025-10-02T10:30:15.345Z	INFO	Starting workers	{"controller": "pod", "worker count": 1}
```

### 4.2 查看 WorkQueue 的 Metrics

```bash
# controller-runtime 会暴露 Prometheus 指标
kubectl port-forward -n aiops-operator-system svc/aiops-operator-controller-manager-metrics-service 8080:8443

curl http://localhost:8080/metrics | grep workqueue

# 输出示例：
# workqueue_adds_total{name="pod"} 42                    # 入队次数
# workqueue_depth{name="pod"} 0                          # 当前队列长度
# workqueue_queue_duration_seconds_bucket{name="pod"}    # 排队时长
# workqueue_work_duration_seconds_bucket{name="pod"}     # 处理时长
# workqueue_retries_total{name="pod"} 3                  # 重试次数
```

### 4.3 调试模式下查看详细日志

**修改 `cmd/main.go:61`：**
```go
opts := zap.Options{
	Development: true,
	Level:       zapcore.DebugLevel,  // ← 添加此行
}
```

**重新部署后查看日志：**
```bash
kubectl logs -f -n aiops-operator-system deploy/aiops-operator-controller-manager

# 会看到详细的 Informer/WorkQueue 日志：
# DEBUG	controller-runtime.source.Kind	Queueing object	{"object": {"name":"bad-pod-abc123","namespace":"default"}}
# DEBUG	controller-runtime.manager.controller.pod	Processing item	{"request": "default/bad-pod-abc123"}
# DEBUG	controller-runtime.manager.controller.pod	Successfully Reconciled	{"duration": "123.456ms"}
```

---

## 🎓 五、与原生 client-go 的对比

### 5.1 原生 client-go 写法（你需要手写所有代码）

```go
import (
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

// 手动创建 Informer
factory := informers.NewSharedInformerFactory(clientset, 0)
podInformer := factory.Core().V1().Pods().Informer()

// 手动创建 WorkQueue
queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())

// 手动注册 EventHandler
podInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
	AddFunc: func(obj interface{}) {
		key, _ := cache.MetaNamespaceKeyFunc(obj)
		queue.Add(key)
	},
	UpdateFunc: func(oldObj, newObj interface{}) {
		key, _ := cache.MetaNamespaceKeyFunc(newObj)
		queue.Add(key)
	},
	DeleteFunc: func(obj interface{}) {
		key, _ := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
		queue.Add(key)
	},
})

// 手动启动 Informer
go factory.Start(stopCh)

// 手动启动 Worker
for i := 0; i < 5; i++ {
	go func() {
		for {
			key, quit := queue.Get()
			if quit {
				return
			}
			
			// 手动调用处理逻辑
			err := processItem(key.(string))
			
			if err != nil {
				queue.AddRateLimited(key)
			} else {
				queue.Forget(key)
			}
			queue.Done(key)
		}
	}()
}
```

### 5.2 controller-runtime 写法（框架帮你搞定一切）

```go
// 仅需 3 行代码！
func (r *PodDetectorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Pod{}).
		Complete(r)
}
```

**框架自动处理：**
- ✅ Informer 创建和启动
- ✅ WorkQueue 创建和配置
- ✅ EventHandler 注册
- ✅ Worker goroutines 启动
- ✅ 错误重试和限流
- ✅ Metrics 暴露

---

## 📍 六、高级用法：显式控制 Informer

### 6.1 监听多个资源

```go
func (r *PodDetectorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Pod{}).              // 主资源（创建 Pod Informer）
		Owns(&v1alpha1.Remediation{}).   // 从属资源（创建 Remediation Informer）
		Watches(                          // 其他资源（手动创建 Event Informer）
			&corev1.Event{},
			handler.EnqueueRequestForOwner(
				mgr.GetScheme(),
				mgr.GetRESTMapper(),
				&corev1.Pod{},
			),
		).
		Complete(r)
}
```

### 6.2 自定义 EventHandler

```go
import "sigs.k8s.io/controller-runtime/pkg/handler"

func (r *PodDetectorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Pod{}).
		Watches(
			&corev1.ConfigMap{},
			handler.EnqueueRequestsFromMapFunc(func(obj client.Object) []reconcile.Request {
				// 自定义逻辑：ConfigMap 变化时触发所有 Pod 的 Reconcile
				podList := &corev1.PodList{}
				_ = r.List(context.Background(), podList)
				
				var reqs []reconcile.Request
				for _, pod := range podList.Items {
					reqs = append(reqs, reconcile.Request{
						NamespacedName: types.NamespacedName{
							Name:      pod.Name,
							Namespace: pod.Namespace,
						},
					})
				}
				return reqs
			}),
		).
		Complete(r)
}
```

### 6.3 调整并发 Worker 数量

```go
func (r *PodDetectorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Pod{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 5,  // ← 同时运行 5 个 Worker
		}).
		Complete(r)
}
```

---

## 🎯 七、总结

| 组件 | 在你代码中的位置 | 由谁创建 |
|------|------------------|----------|
| **Informer** | `SetupWithManager()` 中的 `For()` | controller-runtime 框架自动创建 |
| **WorkQueue** | `SetupWithManager()` 中的 `Complete()` | controller-runtime 框架自动创建 |
| **EventHandler** | 隐式注册 | controller-runtime 框架自动注册 |
| **Worker** | 隐式启动 | controller-runtime 框架自动启动 |
| **你的代码** | `Reconcile(ctx, req)` | 由 Worker 调用 |

**核心理念：**
> **controller-runtime 是对 client-go 的高级封装**，让你专注于业务逻辑（Reconcile），而不需要关心底层的 Informer、WorkQueue、限流等细节。

**如果你需要更细粒度的控制：**
1. **推荐方式：** 使用 `Watches()`、`Owns()`、`WithOptions()` 等高级 API
2. **不推荐：** 直接使用 client-go 的 Informer/WorkQueue（会失去框架的便利性）

---

## 📚 参考资源

- **controller-runtime 架构：** https://github.com/kubernetes-sigs/controller-runtime/blob/main/designs/controller.md
- **client-go Informer：** https://github.com/kubernetes/sample-controller/blob/master/docs/controller-client-go.md
- **Kubebuilder Book：** https://book.kubebuilder.io/cronjob-tutorial/controller-implementation.html
