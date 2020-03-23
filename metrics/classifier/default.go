package classifier

import (
	"context"
	"fmt"
	"time"

	"github.com/quorumcontrol/chaintree/dag"
)

func Default(ctx context.Context, startDag *dag.Dag, endDag *dag.Dag) (classification string, tags []string, err error) {
	ctx2, cancelFn := context.WithTimeout(ctx, 5*time.Second)
	defer cancelFn()

	if startDag == nil {
		tags = append(tags, "new")
	}

	data, _, err := endDag.Resolve(ctx2, []string{"tree", "data"})
	if err != nil {
		return
	}
	if data == nil {
		return
	}

	dataMap, ok := data.(map[string]interface{})
	if !ok {
		err = fmt.Errorf("error casting data attr to map")
		return
	}

	if _, ok := dataMap["dgit"]; ok {
		dgitDataUncast, _, _ := endDag.Resolve(ctx2, []string{"tree", "data", "dgit"})
		if dgitData, ok := dgitDataUncast.(map[string]interface{}); ok && dgitData["repo"] != nil {
			tags = append(tags, fmt.Sprintf("repo=%s", dgitData["repo"]))
		}
		classification = "dgit"
	}

	return
}
