package middleware

import (
	"encoding/json"
	"reflect"

	"github.com/AsynkronIT/protoactor-go/actor"
	"go.uber.org/zap"
)

var Log *zap.SugaredLogger

func init() {
	Log = buildLogger()
}

type LogAware interface {
	SetLog(log *zap.SugaredLogger)
}

type LogAwareHolder struct {
	Log *zap.SugaredLogger
}

func (state *LogAwareHolder) SetLog(log *zap.SugaredLogger) {
	state.Log = log
}

type LogPlugin struct{}

func (p *LogPlugin) OnStart(ctx actor.Context) {
	if p, ok := ctx.Actor().(LogAware); ok {
		p.SetLog(Log.Named(ctx.Self().GetId()))
	}
}
func (p *LogPlugin) OnOtherMessage(ctx actor.Context, usrMsg interface{}) {}

// Logger is message middleware which logs messages before continuing to the next middleware
func LoggingMiddleware(next actor.ActorFunc) actor.ActorFunc {
	fn := func(c actor.Context) {
		message := c.Message()
		Log.Debugw("message", "id", c.Self(), "type", reflect.TypeOf(message), "sender", c.Sender().GetId()) //, "msg", message)
		next(c)
	}

	return fn
}

func buildLogger() *zap.SugaredLogger {
	rawJSON := []byte(`{
		"level": "info",
		"encoding": "console",
		"outputPaths": ["stdout"],
		"errorOutputPaths": ["stderr"],
		"encoderConfig": {
		  "messageKey": "message",
		  "levelKey": "level",
		  "levelEncoder": "lowercase",
		  "nameKey": "name"
		}
	  }`)

	var cfg zap.Config
	if err := json.Unmarshal(rawJSON, &cfg); err != nil {
		panic(err)
	}
	logger, err := cfg.Build()
	if err != nil {
		panic(err)
	}
	return logger.Sugar()
}
