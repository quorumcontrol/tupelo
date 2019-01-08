package middleware

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zapcore"
)

func TestSetLogLevel(t *testing.T) {
	SetLogLevel("error")
	assert.True(t, Log.Desugar().Core().Enabled(zapcore.ErrorLevel))
	assert.False(t, Log.Desugar().Core().Enabled(zapcore.DebugLevel))

	SetLogLevel("debug")
	assert.True(t, Log.Desugar().Core().Enabled(zapcore.ErrorLevel))
	assert.True(t, Log.Desugar().Core().Enabled(zapcore.DebugLevel))
}
