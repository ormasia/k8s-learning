好👌，我们用

```bash
kubectl create deployment redis --image=redis
```

来重做你上面删除 Pod → 自动拉起新 Pod 的实验。

---

## 🔹 实验步骤

### 1. 创建 Deployment

```bash
kubectl create deployment redis --image=redis
```

输出：

```
deployment.apps/redis created
```

---

### 2. 查看资源

```bash
kubectl get all
```

可能输出类似：

```
NAME                         READY   STATUS    RESTARTS   AGE
pod/redis-5b7f9b9dfd-xyz12   1/1     Running   0          20s

NAME                 TYPE        CLUSTER-IP   EXTERNAL-IP   PORT(S)   AGE
service/kubernetes   ClusterIP   10.96.0.1    <none>        443/TCP   3d

NAME                    READY   UP-TO-DATE   AVAILABLE   AGE
deployment.apps/redis   1/1     1            1           20s

NAME                               DESIRED   CURRENT   READY   AGE
replicaset.apps/redis-5b7f9b9dfd   1         1         1       20s
```

👉 和之前的 **单 Pod 版本**不同，这里你能看到：

* 一个 **Deployment（redis）**
* 一个 **ReplicaSet**
* 一个 **Pod（名字带 hash 后缀）**

---

kubectl scale deployment redis --replicas=3
---


### 3. 删除 Pod 测试

```bash
kubectl delete pod <POD_NAME>
```

例如：

```bash
kubectl delete pod redis-5b7f9b9dfd-xyz12
```

输出：

```
pod "redis-5b7f9b9dfd-xyz12" deleted
```

---

### 4. 再次查看

```bash
kubectl get all
```

输出：

```
NAME                         READY   STATUS    RESTARTS   AGE
pod/redis-5b7f9b9dfd-abcd3   1/1     Running   0          5s
```

* 新 Pod 被自动拉起来（名字变了 hash）。
* 这是因为 **Deployment 管理的 ReplicaSet 保证副本数为 1**。
* 删除 Pod → ReplicaSet 发现少了一个 → 自动新建 Pod。

---

✅ **结论**：
用 `kubectl create deployment` 创建的 Redis 是 **自愈的（auto-healing）**，Pod 挂掉会自动重建。
而用 `kubectl run --restart=Never` 创建的 Redis Pod 是 **一次性的**，删掉就没了。

---

要不要我顺便帮你把这个实验扩展一下 👉 **把副本数调成 3**，再删掉 Pod，看看 Deployment 是怎么保持多个副本的？


kubectl delete deployment redis