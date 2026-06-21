// Package httpx provides the standard HTTP response envelope and helpers used
// across all cloud-backend handlers.
//
// Every API response uses a uniform JSON envelope so the generated OpenAPI
// schema stays clean and clients (frontend, generated TS) can rely on a single
// shape:
//
//	{
//	  "code": 200,
//	  "message": "success",
//	  "data": <T | null>
//	}
package httpx

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Envelope is the standard response wrapper. Data is generic in spirit; for
// handlers returning untyped payloads (gin.H) use Data of type any.
type Envelope struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

// OK writes a 200 success envelope wrapping data.
func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Envelope{Code: http.StatusOK, Message: "success", Data: data})
}

// OKWith writes a success envelope with a custom message (used by a few legacy
// endpoints whose message string is part of the contract, e.g. "tool
// registered successfully").
func OKWith(c *gin.Context, message string, data any) {
	c.JSON(http.StatusOK, Envelope{Code: http.StatusOK, Message: message, Data: data})
}

// Created writes a 201 success envelope.
func Created(c *gin.Context, data any) {
	c.JSON(http.StatusCreated, Envelope{Code: http.StatusCreated, Message: "created", Data: data})
}

// Accepted writes a 202 success envelope.
func Accepted(c *gin.Context, data any) {
	c.JSON(http.StatusAccepted, Envelope{Code: http.StatusAccepted, Message: "accepted", Data: data})
}

// Fail writes an error envelope at the given HTTP status. status is also
// mirrored into envelope.Code for consistency with the success path.
func Fail(c *gin.Context, status int, msg string) {
	c.JSON(status, Envelope{Code: status, Message: msg, Data: nil})
}

// FailFrom writes an error envelope, deriving status from err if it implements
// a StatusCoder (gin.Error does); otherwise 500. Convenience for service calls.
func FailFrom(c *gin.Context, defaultStatus int, err error) {
	if err == nil {
		OK(c, nil)
		return
	}
	Fail(c, defaultStatus, err.Error())
}
