package result

type Result struct {
	Type        string
	Did         string
	Tags        []string
	Round       uint64
	LastRound   uint64
	TotalBlocks uint64
	DeltaBlocks uint64
}
