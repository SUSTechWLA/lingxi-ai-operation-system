package builtin

import (
	"context"

	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/worker/tool"
)

type WeatherTool struct{}

func NewWeatherTool() *WeatherTool {
	return &WeatherTool{}
}

func (t *WeatherTool) Name() string       { return "weather" }
func (t *WeatherTool) Description() string { return "Query weather information for a given city" }
func (t *WeatherTool) Type() tool.ToolType { return tool.ToolTypeCustom }

func (t *WeatherTool) Execute(ctx context.Context, params map[string]interface{}, toolCtx tool.ToolContext) tool.ToolResult {
	city, _ := params["city"].(string)
	if city == "" {
		city, _ = params["location"].(string)
	}
	if city == "" {
		return tool.FailureResult("City parameter is required")
	}

	// Simulated weather data - in production this would call a real weather API
	weatherData := map[string]interface{}{
		"city":        city,
		"temperature": "22°C",
		"condition":   "Sunny",
		"humidity":    "45%",
		"wind":        "Light breeze, 8 km/h",
		"forecast":    "Clear skies expected throughout the day",
	}

	return tool.SuccessResult(weatherData)
}

func (t *WeatherTool) ValidateParameters(params map[string]interface{}) bool {
	_, hasCity := params["city"]
	_, hasLocation := params["location"]
	return hasCity || hasLocation
}
