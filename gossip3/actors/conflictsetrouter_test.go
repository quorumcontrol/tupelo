package actors

import (
	"testing"

	"github.com/AsynkronIT/protoactor-go/actor"
)

func TestNewTransaction(t *testing.T) {
	cs := actor.Spawn(NewConflictSetProps("testconflictset1"))
	defer cs.Poison()

}
