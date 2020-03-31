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

type Summary struct {
	Type         string
	Count        uint64
	DeltaCount   uint64
	TotalBlocks  uint64
	DeltaBlocks  uint64
	Round        uint64
	RoundCid     string
	LastRound    uint64
	LastRoundCid string
}
