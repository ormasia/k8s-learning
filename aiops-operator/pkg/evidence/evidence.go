package evidence

import (
	"context"
	"encoding/json"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
		Type          string `json:"type"`
		Reason        string `json:"reason"`
		Message       string `json:"message"`
		Count         int32  `json:"count"`
		LastTimestamp string `json:"lastTimestamp,omitempty"`
	} `json:"events"`
	PreviousLogs string `json:"previousLogs"`
}

func Collect(ctx context.Context, c client.Client, pod *corev1.Pod) ([]byte, error) {
	log := ctrl.LoggerFrom(ctx)
	ev := Evidence{}
	ev.Pod.Namespace = pod.Namespace
	ev.Pod.Name = pod.Name
	ev.Pod.Phase = pod.Status.Phase
	ev.Pod.ContainerStatuses = pod.Status.ContainerStatuses
	for _, con := range pod.Spec.Containers {
		ev.Pod.Images = append(ev.Pod.Images, map[string]string{"name": con.Name, "image": con.Image})
	}

	// 列出同命名空间的 Events，然后在内存中过滤到与该 Pod 相关的事件
	// （说明：controller-runtime 的缓存 List 想用 fieldSelector 需要额外索引；否则退回内存过滤或使用 clientset 直连 API Server）
	var list corev1.EventList
	if err := c.List(ctx, &list, client.InNamespace(pod.Namespace)); err == nil {
		for _, e := range list.Items {
			if e.InvolvedObject.Kind == "Pod" &&
				e.InvolvedObject.Name == pod.Name &&
				e.InvolvedObject.Namespace == pod.Namespace {
				ev.Events = append(ev.Events, struct {
					Type          string `json:"type"`
					Reason        string `json:"reason"`
					Message       string `json:"message"`
					Count         int32  `json:"count"`
					LastTimestamp string `json:"lastTimestamp,omitempty"`
				}{
					Type:          e.Type,
					Reason:        e.Reason,
					Message:       e.Message,
					Count:         e.Count,
					LastTimestamp: RFC3339(metav1.Time{Time: e.EventTime.Time}),
				})
			}
		}
	}

	// previous 日志：此处先留空。若要取 --previous 日志，请在 manager 中注入 clientset，
	// 使用 CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, &PodLogOptions{Previous:true, Container:<name>})
	// 将结果填到 ev.PreviousLogs。
	ev.PreviousLogs = ""

	// 序列化
	b, err := json.Marshal(ev)
	if err != nil {
		log.Error(err, "marshal evidence failed")
		return nil, err
	}
	return b, nil
}

// 一个简易异常判定（根据 Waiting.Reason）
func IsAnomalous(pod *corev1.Pod) bool {
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.State.Waiting != nil {
			r := cs.State.Waiting.Reason
			if r == "ImagePullBackOff" || r == "ErrImagePull" || r == "CrashLoopBackOff" {
				return true
			}
		}
	}
	return false
}

func RFC3339(t metav1.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Time.UTC().Format(time.RFC3339)
}
