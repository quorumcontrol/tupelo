package middleware

import (
	"reflect"

	"github.com/AsynkronIT/protoactor-go/actor"
	"go.uber.org/zap"
)

var Log *zap.SugaredLogger

func init() {
	logger, err := zap.NewDevelopment()
	if err != nil {
		panic("error")
	}
	Log = logger.Sugar()
}

// Logger is message middleware which logs messages before continuing to the next middleware
func LoggingMiddleware(next actor.ActorFunc) actor.ActorFunc {
	fn := func(c actor.Context) {
		message := c.Message()
		Log.Infow("message", "id", c.Self(), "type", reflect.TypeOf(message), "sender", c.Sender().GetId()) //, "msg", message)
		next(c)
	}

	return fn
}
