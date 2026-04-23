package logger

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func Init(mode string) {
	var cfg zap.Config
	if mode == "production" {
		cfg = zap.NewProductionConfig()
	} else {
		cfg = zap.NewDevelopmentConfig()
		cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}

	cfg.EncoderConfig.TimeKey = "timestamp"
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	logger, err := cfg.Build()
	if err != nil {
		panic("failed to init logger: " + err.Error())
	}

	zap.ReplaceGlobals(logger)
}
