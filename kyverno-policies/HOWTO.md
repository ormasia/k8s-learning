# 📚 Kyverno 策略编写与使用完全指南

## 🎯 一、你的项目中 Kyverno 的安装方式

### 1.1 通过 Helm 安装（已完成）

```bash
# 第一步：安装 Kyverno 引擎
helm repo add kyverno https://kyverno.github.io/kyverno/
helm install kyverno kyverno/kyverno -n kyverno --create-namespace

# 第二步：安装官方策略库（12 个预定义策略）
helm install kyverno-policies kyverno/kyverno-policies -n kyverno
```

**安装记录：**
```
$ helm list -A | grep kyverno
kyverno                 kyverno         1       2025-09-30 18:28:12     deployed        kyverno-3.5.2           v1.15.2
kyverno-policies        kyverno         1       2025-09-30 18:29:22     deployed        kyverno-policies-3.5.2  v1.15.2
```

---

## 📝 二、Kyverno 策略结构详解

### 2.1 策略的基本结构

```yaml
apiVersion: kyverno.io/v1
kind: ClusterPolicy              # 集群级策略（所有命名空间生效）
# 或 kind: Policy                # 命名空间级策略（仅特定命名空间生效）

metadata:
  name: disallow-latest-tag      # 策略名称
  annotations:
    policies.kyverno.io/title: "Disallow Latest Tag"
    policies.kyverno.io/category: "Best Practices"
    policies.kyverno.io/severity: "medium"
    policies.kyverno.io/description: "禁止使用 :latest 标签"

spec:
  # ========== 全局配置 ==========
  validationFailureAction: Enforce  # Enforce=拒绝 / Audit=允许但记录
  background: true                  # 是否扫描现有资源
  
  # ========== 规则列表 ==========
  rules:
  - name: require-image-tag        # 规则名称
    
    # 匹配条件：对哪些资源生效
    match:
      any:
      - resources:
          kinds:
          - Pod                    # 对所有 Pod 生效
    
    # 验证逻辑
    validate:
      message: "An image tag is required."  # 失败时的提示信息
      
      # foreach 遍历多个字段
      foreach:
      - list: request.object.spec.containers      # 遍历所有容器
        pattern:
          image: "*:*"             # 必须包含标签（格式：镜像名:标签）
      
      - list: request.object.spec.initContainers  # 遍历 initContainers
        pattern:
          image: "*:*"
      
      - list: request.object.spec.ephemeralContainers
        pattern:
          image: "*:*"
  
  # 第二条规则：禁止 latest 标签
  - name: validate-image-tag
    match:
      any:
      - resources:
          kinds:
          - Pod
    
    validate:
      message: "Using a mutable image tag e.g. 'latest' is not allowed."
      foreach:
      - list: request.object.spec.containers
        pattern:
          image: "!*:latest"       # 不允许 :latest（! 表示否定）
      
      - list: request.object.spec.initContainers
        pattern:
          image: "!*:latest"
      
      - list: request.object.spec.ephemeralContainers
        pattern:
          image: "!*:latest"
```

---

## 🔍 三、策略的 4 种类型

### 3.1 Validate（验证）- 最常用

**作用：** 验证资源是否符合规范，不符合则拒绝或记录

```yaml
spec:
  rules:
  - name: check-image-tag
    validate:
      pattern:
        spec:
          containers:
          - image: "!*:latest"     # 禁止 latest
```

### 3.2 Mutate（变更）- 自动修改

**作用：** 自动修改资源配置

```yaml
spec:
  rules:
  - name: add-default-resources
    mutate:
      patchStrategicMerge:
        spec:
          containers:
          - (name): "*"            # 所有容器
            resources:
              limits:
                memory: "512Mi"    # 自动添加资源限制
```

### 3.3 Generate（生成）- 自动创建资源

**作用：** 在创建某个资源时，自动创建关联资源

```yaml
spec:
  rules:
  - name: create-configmap
    match:
      any:
      - resources:
          kinds:
          - Namespace
    generate:
      kind: ConfigMap           # 自动创建 ConfigMap
      name: default-config
      data:
        key: value
```

### 3.4 Verify Images（镜像验证）- 签名验证

**作用：** 验证容器镜像签名

```yaml
spec:
  rules:
  - name: verify-signature
    verifyImages:
    - imageReferences:
      - "ghcr.io/myorg/*"
      attestors:
      - count: 1
        entries:
        - keys:
            publicKeys: |-
              -----BEGIN PUBLIC KEY-----
              ...
```

---

## 📍 四、如何自己编写策略

### 4.1 方法 1：从官方策略库选择

**Kyverno 官方提供 150+ 策略：**
- 网站：https://kyverno.io/policies/
- 分类：
  - Pod Security Standards (PSS)
  - Best Practices
  - Security
  - Multi-Tenancy
  - Argo

**使用方法：**
```bash
# 1. 访问官方策略库
https://kyverno.io/policies/

# 2. 选择需要的策略，复制 YAML

# 3. 应用到集群
kubectl apply -f policy.yaml
```

### 4.2 方法 2：从零开始手写

**步骤：**

1. **创建策略文件**
```bash
cat <<EOF > my-policy.yaml
apiVersion: kyverno.io/v1
kind: ClusterPolicy
metadata:
  name: require-labels
spec:
  validationFailureAction: Enforce
  rules:
  - name: check-for-labels
    match:
      any:
      - resources:
          kinds:
          - Pod
    validate:
      message: "Label 'app' is required."
      pattern:
        metadata:
          labels:
            app: "?*"              # 必须有 app 标签
EOF
```

2. **验证语法**
```bash
# 使用 kubectl 验证
kubectl apply --dry-run=server -f my-policy.yaml
```

3. **应用策略**
```bash
kubectl apply -f my-policy.yaml
```

4. **测试策略**
```bash
# 测试没有 app 标签的 Pod（应该被拒绝）
cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Pod
metadata:
  name: test-pod
spec:
  containers:
  - name: nginx
    image: nginx:1.19
EOF
# 预期输出：Error: Label 'app' is required.

# 测试有 app 标签的 Pod（应该成功）
cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Pod
metadata:
  name: test-pod
  labels:
    app: myapp          # ✅ 有 app 标签
spec:
  containers:
  - name: nginx
    image: nginx:1.19
EOF
```

### 4.3 方法 3：使用 Kyverno CLI 生成

```bash
# 安装 Kyverno CLI
kubectl krew install kyverno

# 测试策略
kyverno apply my-policy.yaml --resource test-pod.yaml
```

---

## 🎯 五、你的项目中的 12 个策略详解

### 5.1 策略清单

| 策略名 | 类型 | 作用 | 严重性 |
|--------|------|------|--------|
| **disallow-latest-tag** | Validate | 禁止 :latest 标签 | Medium |
| disallow-privileged-containers | Validate | 禁止特权容器 | High |
| disallow-host-namespaces | Validate | 禁止使用主机命名空间 | High |
| disallow-host-path | Validate | 禁止挂载主机路径 | High |
| disallow-host-ports | Validate | 禁止使用主机端口 | Medium |
| disallow-host-process | Validate | 禁止主机进程容器 | High |
| disallow-capabilities | Validate | 限制 Linux capabilities | Medium |
| disallow-proc-mount | Validate | 禁止修改 /proc 挂载 | Medium |
| disallow-selinux | Validate | 限制 SELinux 选项 | Medium |
| restrict-apparmor-profiles | Validate | 限制 AppArmor 配置 | Medium |
| restrict-seccomp | Validate | 限制 Seccomp 配置 | Medium |
| restrict-sysctls | Validate | 限制系统调用参数 | Medium |

### 5.2 为什么选择这 12 个策略？

**这是 Kyverno 官方的 `baseline` 级别策略集：**

```bash
# 查看 Helm Chart 配置
helm show values kyverno/kyverno-policies

# 默认配置
podSecurityStandard: baseline    # 基线安全级别
validationFailureAction: Audit   # 审计模式
```

**三个安全级别：**
1. **privileged**（特权）- 无限制（0 个策略）
2. **baseline**（基线）- 基本安全（12 个策略）← 你安装的
3. **restricted**（严格）- 高度限制（20+ 策略）

---

## 🛠️ 六、如何修改策略行为

### 6.1 修改全局模式（Audit ↔ Enforce）

```bash
# 方法1：通过 Helm 升级
helm upgrade kyverno-policies kyverno/kyverno-policies \
  -n kyverno \
  --set validationFailureAction=Enforce

# 方法2：直接编辑策略
kubectl edit clusterpolicy disallow-latest-tag

# 修改这一行：
spec:
  validationFailureAction: Enforce  # 改为 Enforce
```

### 6.2 为特定策略设置不同模式

```bash
# 仅对 latest-tag 策略使用 Enforce
helm upgrade kyverno-policies kyverno/kyverno-policies \
  -n kyverno \
  --set validationFailureActionByPolicy.disallow-latest-tag=Enforce
```

### 6.3 添加例外（PolicyException）

```yaml
apiVersion: kyverno.io/v2
kind: PolicyException
metadata:
  name: allow-latest-for-dev
  namespace: kyverno
spec:
  exceptions:
  - policyName: disallow-latest-tag
    ruleNames:
    - validate-image-tag
  match:
    any:
    - resources:
        namespaces:
        - dev                 # 仅在 dev 命名空间允许 :latest
```

---

## 📊 七、策略效果验证

### 7.1 查看策略报告

```bash
# 查看策略违规报告
kubectl get policyreport -A

# 查看具体报告
kubectl get policyreport -n default -o yaml
```

### 7.2 测试策略

```bash
# 测试1：创建违规 Pod（应该被拒绝或警告）
kubectl run test --image=nginx:latest

# 测试2：创建合规 Pod（应该成功）
kubectl run test --image=nginx:1.19
```

### 7.3 查看策略状态

```bash
# 查看策略是否就绪
kubectl get clusterpolicy

# 查看策略详情
kubectl describe clusterpolicy disallow-latest-tag
```

---

## 🎓 八、总结

### 8.1 你的项目使用 Kyverno 的步骤

1. ✅ **安装 Kyverno 引擎**（Helm）
2. ✅ **安装官方策略库**（Helm - baseline 级别）
3. ✅ **策略自动生效**（无需手写，官方预定义）
4. ✅ **集成 aiops-operator**（异常检测 + 自动修复）

### 8.2 策略来源

- **官方策略库**: https://kyverno.io/policies/
- **GitHub**: https://github.com/kyverno/kyverno/tree/main/charts/kyverno-policies
- **Helm Chart**: `kyverno/kyverno-policies`

### 8.3 如何自定义策略

**三种方法：**
1. ✅ **使用官方策略**（推荐，150+ 预定义）
2. ✅ **修改官方策略**（基于已有策略调整）
3. ✅ **从零编写**（参考官方文档和示例）

**编写流程：**
```
1. 定义策略目标 → 2. 选择策略类型（Validate/Mutate/Generate）
   ↓
3. 编写 YAML → 4. 测试验证 → 5. 应用到集群
```

---

## 📚 九、参考资源

- **官方文档**: https://kyverno.io/docs/
- **策略库**: https://kyverno.io/policies/
- **GitHub**: https://github.com/kyverno/kyverno
- **社区**: https://slack.k8s.io/ (#kyverno)

---

## 💡 十、常见问题

**Q1: 策略太严格，如何放宽？**
```bash
# 改为 Audit 模式（仅警告，不拒绝）
kubectl patch clusterpolicy disallow-latest-tag \
  --type=merge \
  -p '{"spec":{"validationFailureAction":"Audit"}}'
```

**Q2: 如何查看哪些 Pod 违规？**
```bash
kubectl get policyreport -A
```

**Q3: 如何删除策略？**
```bash
kubectl delete clusterpolicy disallow-latest-tag
```

**Q4: 如何卸载 Kyverno？**
```bash
helm uninstall kyverno-policies -n kyverno
helm uninstall kyverno -n kyverno
kubectl delete ns kyverno
```
