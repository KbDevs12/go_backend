package response

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
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

func OK(c *gin.Context, message string, data interface{}) {
	c.JSON(http.StatusOK, APIResponse{Success: true, Message: message, Data: data})
}

func Created(c *gin.Context, message string, data interface{}) {
	c.JSON(http.StatusCreated, APIResponse{Success: true, Message: message, Data: data})
}

func BadRequest(c *gin.Context, message string) {
	c.JSON(http.StatusBadRequest, APIResponse{Success: false, Message: message, Data: nil})
}

func Unauthorized(c *gin.Context, message string) {
	c.JSON(http.StatusUnauthorized, APIResponse{Success: false, Message: message, Data: nil})
}

func Forbidden(c *gin.Context, message string) {
	c.JSON(http.StatusForbidden, APIResponse{Success: false, Message: message, Data: nil})
}

func NotFound(c *gin.Context, message string) {
	c.JSON(http.StatusNotFound, APIResponse{Success: false, Message: message, Data: nil})
}

func InternalError(c *gin.Context, message string) {
	c.JSON(http.StatusInternalServerError, APIResponse{Success: false, Message: message, Data: nil})
}

func Conflict(c *gin.Context, message string) {
	c.JSON(http.StatusConflict, APIResponse{Success: false, Message: message, Data: nil})
}
