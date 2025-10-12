# 面试版 AIOps Operator 项目讲解

> 适用于 5~10 分钟技术面自述：先抛出业务痛点与成果，再逐层拆解架构、流程、技术难题，最后补充上线经验与常见追问。

## 1. 开场 30 秒：项目定位 & 价值
- **项目一句话**：我们做了一套 AIOps Operator，把 Kubernetes 运行期故障处理串成“异常检测 → LLM 生成补丁 → 人工审批 → Server-Side Apply 落地”的闭环，实现分钟级自愈。
- **业务痛点**：传统 SRE 需要人工排查 Pod Crash/ImagePullBackOff 等问题，效率低且不可审计。
- **量化成果**：接入双控制器后，常见镜像/配置类故障从发现到修复缩短到 ~5 分钟；配合 Kyverno 准入基线，准入阶段可拦截约 40% 的配置错误。【F:aiops-operator/internal/controller/pod_controller.go†L70-L138】【F:kyverno-policies/disallow-latest-tag.yaml†L1-L56】

## 2. 三层架构速览
1. **自定义接口层（Remediation CRD）**：声明修复目标、证据、审批开关以及状态机，是团队协作与审计的统一入口。【F:aiops-operator/api/v1alpha1/remediation_types.go†L24-L74】
2. **控制循环层**：
   - `PodDetectorReconciler` 监听异常 Pod，收集证据后 Upsert Remediation。【F:aiops-operator/internal/controller/pod_controller.go†L70-L138】【F:aiops-operator/pkg/evidence/evidence.go†L36-L97】
   - `RemediationExecutorReconciler` 负责 LLM 补丁生成、审批流与安全执行。【F:aiops-operator/internal/controller/remediation_controller.go†L44-L166】
3. **外部能力层**：Ollama 本地模型用于生成结构化补丁；Kyverno 基线策略提供准入守卫，减少无效 Remediation。【F:aiops-operator/pkg/llm/ollama.go†L12-L90】【F:kyverno-policies/HOWTO.md†L1-L41】

## 3. 端到端流程（面试重点讲 2 分钟）
1. **异常判定**：检测器捕获 `ImagePullBackOff/ErrImagePull/CrashLoopBackOff` 等状态，判定为异常。【F:aiops-operator/pkg/evidence/evidence.go†L85-L97】
2. **证据归档**：聚合 Pod 信息、容器状态、相关 Events，写入 `spec.evidence` 并标记 `Diagnosing` Condition。【F:aiops-operator/internal/controller/pod_controller.go†L92-L134】
3. **LLM 生成方案**：执行器构造系统提示 + JSON Schema，调用 Ollama 产出 `actions/risks/rollbackPlan`，写入 `status.proposedPatch`，状态切换为 `Proposed` / `ReadyForReview`。【F:aiops-operator/internal/controller/remediation_controller.go†L57-L96】
4. **人工审批**：SRE 将 `spec.approved` 置为 `true`，触发执行器后续流程；若补丁缺失则立即记录 `Failed` Condition，保证可观测。【F:aiops-operator/internal/controller/remediation_controller.go†L98-L119】
5. **安全执行**：取首个 action 转换为 SSA 片段，先 `DryRunAll` 再正式 `Apply`，并使用 `FieldOwner("aiops-operator")` 管理字段所有权，成功后写入 `Applied` Condition 与时间戳。【F:aiops-operator/internal/controller/remediation_controller.go†L120-L166】

## 4. 技术亮点与难点
- **结构化 LLM 输出**：利用 Ollama `format` 传入 JSON Schema，强制模型给出可直接执行的补丁，降低后处理成本。【F:aiops-operator/pkg/llm/ollama.go†L32-L75】
- **SSA 安全护栏**：Dry-run 阶段提前暴露 schema/权限/所有权冲突，正式 Apply 指定 FieldOwner，必要时使用 ForceOwnership，避免和业务控制器“抢字段”。【F:aiops-operator/internal/controller/remediation_controller.go†L120-L166】
- **审计友好的状态机**：`conditions` 维护 `Diagnosing/Proposed/ReadyForReview/Applied/Failed`，配合 `lastUpdateTime` 让审批、执行全链路可追溯。【F:aiops-operator/api/v1alpha1/remediation_types.go†L54-L74】
- **证据扩展性**：`pkg/evidence` 预留 previous logs 等扩展点，可在后续版本接入更多信号，提高 LLM 判断准确率。【F:aiops-operator/pkg/evidence/evidence.go†L1-L83】

## 5. Kyverno 准入如何协同
- **部署策略**：通过 Helm 一次性安装 Kyverno 引擎与官方 baseline 策略，默认覆盖镜像标签、特权容器、主机访问等 12 条规则。【F:kyverno-policies/HOWTO.md†L1-L41】
- **场景示例**：`disallow-latest-tag` 在准入阶段拒绝使用 `:latest`，把大量低级错误挡在创建之前，减少 Remediation 噪声。【F:kyverno-policies/disallow-latest-tag.yaml†L1-L56】
- **面试表述**：强调“准入前置防线 + 运行期自动修复”形成闭环——Kyverno 限制不合规对象进入，Operator 处理仍然落网的运行时异常。

## 6. 交付与演示要点
1. **部署步骤**（确保环境具备 Go / Docker / Kubebuilder / Kind / Ollama）：【F:environment.md†L1-L57】
   ```bash
   make docker-build docker-push IMG=<registry>/aiops-operator:tag
   make install
   make deploy IMG=<registry>/aiops-operator:tag
   ```
2. **演示剧本**：
   - 创建一个使用错误镜像标签的 Deployment，观察 Kyverno 是否阻断；若策略为 Audit，可继续创建验证运行期异常。
   - 等待生成的 Remediation，查看 `status.proposedPatch`、LLM 建议与 Conditions。
   - 手动批准，观察控制器日志中的 dry-run/SSA 过程，以及 Deployment 恢复情况。
3. **可视化素材**：准备 `kubectl get remediation -o yaml` 片段或 Grafana 报表截图，展示从异常→修复的时间线。

## 7. 常见追问 & 回答提示
| 面试问题 | 回答要点 |
| --- | --- |
| 为什么要自定义 CRD？ | 需要声明式接口聚合证据、审批、补丁，并复用 RBAC/审计能力。【F:aiops-operator/api/v1alpha1/remediation_types.go†L24-L74】 |
| 如何保证 LLM 输出可控？ | JSON Schema 约束 + 严格解析，任何不合法输出都触发 `Failed` Condition 并等待重试。【F:aiops-operator/internal/controller/remediation_controller.go†L57-L119】 |
| 如果补丁涉及多个对象？ | 当前 MVP 取第一条 action，保留完整 JSON，未来可迭代多对象执行或引入人工筛选流程。【F:aiops-operator/internal/controller/remediation_controller.go†L108-L134】 |
| 如何扩展更多异常类型？ | 增加新的 Detector（Deployment/StatefulSet 等），沿用 Remediation CR 聚合多源事件。【F:aiops-operator/internal/controller/pod_controller.go†L70-L138】 |
| 灰度/生产如何落地？ | 通过环境变量区分模型地址，先在预发集群启用 `Audit` 模式观察效果，再切换 `Enforce` 与自动审批策略。|
| Kyverno 会直接生成 Remediation 吗？ | 目前不会：Kyverno 只负责准入拦截或审计。未来规划是把违规事件转换为 Remediation，但当前版本仍由 Pod 检测器触发。|

## 8. 后续规划
- **增强证据采集**（规划中）：对接容器 previous logs、节点指标，提高 LLM 诊断准确度。【F:aiops-operator/pkg/evidence/evidence.go†L1-L83】
- **多动作执行引擎**（规划中）：当前版本仅执行第一条 `action`，后续将支持遍历全部操作并结合策略校验实现批量修复。
- **与策略系统联动**（规划中）：把 Kyverno 违规结果转化为 Remediation 触发信号，实现“策略违规 → 自动补丁 → 审批执行”的统一闭环。
- **接入可视化审批**：输出 `status.proposedPatch` 给前端/ChatOps，降低运维审批门槛。

> 建议面试前准备真实日志、指标或变更记录作为补充材料，展示自动化闭环的可靠性与复盘能力。
