package utils

import (
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// NewLogger creates a new configured logger
func NewLogger() zerolog.Logger {
	// Configure the logger
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	zerolog.SetGlobalLevel(zerolog.InfoLevel)

	if os.Getenv("APP_ENV") == "development" {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}

	// Create a logger that prints a human-friendly format in development
	var logger zerolog.Logger
	if os.Getenv("APP_ENV") == "development" {
		logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339})
	} else {
		logger = log.Logger
	}

	return logger
}

// LoggerMiddleware returns a Gin middleware for logging HTTP requests
func LoggerMiddleware(logger zerolog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery

		// Process request
		c.Next()

		// After request is processed
		param := gin.LogFormatterParams{
			Request: c.Request,
			Keys:    c.Keys,
		}

		param.TimeStamp = time.Now()
		param.Latency = param.TimeStamp.Sub(start)

		if raw != "" {
			path = path + "?" + raw
		}

		statusCode := c.Writer.Status()
		method := c.Request.Method
		clientIP := c.ClientIP()
		errorMessage := c.Errors.ByType(gin.ErrorTypePrivate).String()

		log := logger.With().
			Str("method", method).
			Str("path", path).
			Int("status", statusCode).
			Str("ip", clientIP).
			Dur("latency", param.Latency).
			Logger()

		switch {
		case statusCode >= 500:
			log.Error().Msg(errorMessage)
		case statusCode >= 400:
			log.Warn().Msg(errorMessage)
		default:
			log.Info().Msg("")
		}
	}
}
