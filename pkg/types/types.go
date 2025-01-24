package types

import (
    "encoding/json"
    v1 "k8s.io/api/core/v1"
    networkingv1 "k8s.io/api/networking/v1"
)

type CNIRequest struct {
    Command     string          `json:"command"`
    ContainerID string          `json:"container_id"`
    Netns       string          `json:"netns"`
    IfName      string          `json:"ifname"`
    Config      json.RawMessage `json:"config"`
}

type CNIResponse struct {
    Success  bool            `json:"success"`
    ErrorMsg string          `json:"error_msg,omitempty"`
    Result   json.RawMessage `json:"result,omitempty"`
}

type PodEvent struct {
    Command string  `json:"command"`
    Event   string  `json:"event"`
    Pod     *v1.Pod `json:"pod"`
}

type ReconcileRequest struct {
    Command  string                          `json:"command"`
    Pods     *v1.PodList                     `json:"pods"`
    Policies *networkingv1.NetworkPolicyList `json:"policies"`
}
