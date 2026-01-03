package logging

import (
	"os"
	"strings"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	globalLogger *zap.Logger
	once         sync.Once
)

func Init() {
	once.Do(func() {
		level := parseLevel(os.Getenv("LOG_LEVEL"))

		config := zap.NewProductionEncoderConfig()
		config.TimeKey = "time"
		config.EncodeTime = zapcore.ISO8601TimeEncoder
		config.EncodeLevel = zapcore.CapitalLevelEncoder

		var encoder zapcore.Encoder
		if os.Getenv("LOG_FORMAT") == "json" {
			encoder = zapcore.NewJSONEncoder(config)
		} else {
			config.EncodeLevel = zapcore.CapitalColorLevelEncoder
			encoder = zapcore.NewConsoleEncoder(config)
		}

		core := zapcore.NewCore(
			encoder,
			zapcore.AddSync(os.Stdout),
			level,
		)

		globalLogger = zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1))
	})
}

func parseLevel(levelStr string) zapcore.Level {
	switch strings.ToLower(levelStr) {
	case "debug":
		return zapcore.DebugLevel
	case "info":
		return zapcore.InfoLevel
	case "warn", "warning":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	default:
		return zapcore.InfoLevel
	}
}

func Logger() *zap.Logger {
	if globalLogger == nil {
		Init()
	}
	return globalLogger
}

func Named(name string) *zap.Logger {
	return Logger().Named(name)
}

func Sync() {
	if globalLogger != nil {
		_ = globalLogger.Sync()
	}
}

func Debug(msg string, fields ...zap.Field) {
	Logger().Debug(msg, fields...)
}

func Info(msg string, fields ...zap.Field) {
	Logger().Info(msg, fields...)
}

func Warn(msg string, fields ...zap.Field) {
	Logger().Warn(msg, fields...)
}

func Error(msg string, fields ...zap.Field) {
	Logger().Error(msg, fields...)
}

func Fatal(msg string, fields ...zap.Field) {
	Logger().Fatal(msg, fields...)
}

func With(fields ...zap.Field) *zap.Logger {
	return Logger().With(fields...)
}
