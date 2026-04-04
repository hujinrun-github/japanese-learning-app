package httputil

import (
	"encoding/json"
	"log"
	"net/http"
)

// APIResponse 成功响应包装
type APIResponse struct {
	Data      any    `json:"data"`
	RequestID string `json:"request_id"`
}

// APIError 错误响应
type APIError struct {
	Code      string `json:"code"`       // 业务错误码，如 "ERR_WORD_NOT_FOUND"
	Message   string `json:"message"`    // 用户可读消息
	RequestID string `json:"request_id"` // 用于日志追踪
}

// WriteJSON 将 v 序列化为 JSON 并写入响应，status 为 HTTP 状态码。
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("httputil: failed to encode JSON response: %v", err)
	}
}

// WriteError 写入标准错误响应。
func WriteError(w http.ResponseWriter, status int, code, message, requestID string) {
	WriteJSON(w, status, APIError{
		Code:      code,
		Message:   message,
		RequestID: requestID,
	})
}
