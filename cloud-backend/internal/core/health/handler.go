package health

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/IBM/sarama"
	"github.com/gin-gonic/gin"
)

type DependencyCheck struct {
	Name  string
	Check func(context.Context) error
}

type Handler struct {
	checks  []DependencyCheck
	timeout time.Duration
}

func NewHandler(checks []DependencyCheck) *Handler {
	return &Handler{
		checks:  checks,
		timeout: 2 * time.Second,
	}
}

func (h *Handler) RegisterRoutes(r *gin.Engine) {
	api := r.Group("/api")
	{
		api.GET("/health/ready", h.Ready)
	}
}

func (h *Handler) Ready(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), h.timeout)
	defer cancel()

	httpStatus := http.StatusOK
	status := "UP"
	dependencies := make(map[string]map[string]string, len(h.checks))
	for _, check := range h.checks {
		result := map[string]string{"status": "UP"}
		if err := check.Check(ctx); err != nil {
			httpStatus = http.StatusServiceUnavailable
			status = "DOWN"
			result["status"] = "DOWN"
			result["error"] = err.Error()
		}
		dependencies[check.Name] = result
	}

	c.JSON(httpStatus, gin.H{
		"status":       status,
		"service":      "tangying-ai-os",
		"dependencies": dependencies,
	})
}

func KafkaCheck(bootstrapServers string) func(context.Context) error {
	brokers := strings.Split(bootstrapServers, ",")
	return func(ctx context.Context) error {
		cfg := sarama.NewConfig()
		cfg.Net.DialTimeout = 2 * time.Second
		cfg.Net.ReadTimeout = 2 * time.Second
		cfg.Net.WriteTimeout = 2 * time.Second

		result := make(chan error, 1)
		go func() {
			client, err := sarama.NewClient(brokers, cfg)
			if err != nil {
				result <- err
				return
			}
			defer client.Close()
			_, err = client.Controller()
			result <- err
		}()

		select {
		case err := <-result:
			return err
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
