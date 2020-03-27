package result

type Result struct {
	Type        string
	Did         string
	Round       uint64
	LastRound   uint64
	TotalBlocks uint64
	DeltaBlocks uint64
	Tags        map[string]interface{}
}
