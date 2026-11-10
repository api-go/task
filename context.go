package task

import (
	"context"

	"github.com/api-go/log"
)

type loggerContextKey struct{}

// WithLogger attaches the logger for the current task run to a context.
func WithLogger(ctx context.Context, logger *log.Logger) context.Context {
	return context.WithValue(ctx, loggerContextKey{}, logger)
}

// Logger returns the logger attached to a task context, or the default logger.
func Logger(ctx context.Context) *log.Logger {
	if ctx != nil {
		if logger, ok := ctx.Value(loggerContextKey{}).(*log.Logger); ok && logger != nil {
			return logger
		}
	}
	return log.DefaultLogger
}
