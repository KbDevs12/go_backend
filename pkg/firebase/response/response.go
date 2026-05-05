package response

import (
	"encoding/json"
	"net/http"
)

type APIResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

type PaginatedResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
	Meta    PaginMeta   `json:"meta"`
}

type PaginMeta struct {
	Page       int `json:"page"`
	Limit      int `json:"limit"`
	TotalItems int `json:"total_items"`
	TotalPages int `json:"total_pages"`
}

func JSON(w http.ResponseWriter, statusCode int, success bool, message string, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(APIResponse{
		Success: success,
		Message: message,
		Data:    data,
	})
}

func OK(w http.ResponseWriter, message string, data interface{}) {
	JSON(w, http.StatusOK, true, message, data)
}

func Created(w http.ResponseWriter, message string, data interface{}) {
	JSON(w, http.StatusCreated, true, message, data)
}

func BadRequest(w http.ResponseWriter, message string) {
	JSON(w, http.StatusBadRequest, false, message, nil)
}

func Unauthorized(w http.ResponseWriter, message string) {
	JSON(w, http.StatusUnauthorized, false, message, nil)
}

func Forbidden(w http.ResponseWriter, message string) {
	JSON(w, http.StatusForbidden, false, message, nil)
}

func NotFound(w http.ResponseWriter, message string) {
	JSON(w, http.StatusNotFound, false, message, nil)
}

func InternalError(w http.ResponseWriter, message string) {
	JSON(w, http.StatusInternalServerError, false, message, nil)
}

func Conflict(w http.ResponseWriter, message string) {
	JSON(w, http.StatusConflict, false, message, nil)
}
