package gossip

import "github.com/quorumcontrol/tupelo-go-sdk/gossip/types"

var ZeroVoteID = "n"

type Vote struct {
	Checkpoint *types.WrappedCheckpoint
	tallyCount float64
	id         string
}

func (v *Vote) Nil() {
	v.tallyCount = 0.0
	v.id = ZeroVoteID
}

func (v *Vote) ID() string {
	if v.Checkpoint == nil {
		return ZeroVoteID
	}
	if v.id == "" {
		v.id = v.Checkpoint.CID().String()
	}
	return v.id
}

func (v *Vote) Tally() float64 {
	return v.tallyCount
}

func (v *Vote) SetTally(n float64) {
	v.tallyCount = n
}

func (v *Vote) Length() float64 {
	if v.Checkpoint == nil {
		return 0
	}
	return float64(v.Checkpoint.Length())
}

// Return back the votes with their tallies calculated.
func calculateTallies(responses []*Vote) []*Vote {
	votes := make(map[string]*Vote, len(responses))

	for _, res := range responses {
		snowlog.Debugf("calculateTallies: %s", res.ID())
		vote, exists := votes[res.ID()]
		if !exists {
			vote = res

			votes[vote.ID()] = vote
		}

		vote.SetTally(vote.Tally() + 1.0/float64(len(responses)))
	}

	for id, weight := range Normalize(ComputeProfitWeights(responses)) {
		votes[id].SetTally(votes[id].Tally() * weight)
	}

	totalTally := float64(0)
	for _, checkpoint := range votes {
		totalTally += checkpoint.Tally()
	}

	array := make([]*Vote, 0, len(votes))

	for id := range votes {
		votes[id].SetTally(votes[id].Tally() / totalTally)

		array = append(array, votes[id])
	}

	return array
}

func ComputeProfitWeights(responses []*Vote) map[string]float64 {
	weights := make(map[string]float64, len(responses))

	var max float64

	for _, res := range responses {
		if res.ID() == ZeroVoteID {
			continue
		}

		weights[res.ID()] += res.Length()

		if weights[res.ID()] > max {
			max = weights[res.ID()]
		}
	}

	for id := range weights {
		weights[id] /= max
	}

	return weights
}

func Normalize(weights map[string]float64) map[string]float64 {
	normalized := make(map[string]float64, len(weights))
	min, max := float64(1), float64(0)

	// Find minimum weight.
	for _, weight := range weights {
		if min > weight {
			min = weight
		}
	}

	// Subtract minimum and find maximum normalized weight.
	for vote, weight := range weights {
		normalized[vote] = weight - min

		if normalized[vote] > max {
			max = normalized[vote]
		}
	}

	// Normalize weight using maximum normalized weight into range [0, 1].
	for vote := range weights {
		if max == 0 {
			normalized[vote] = 1
		} else {
			normalized[vote] /= max
		}
	}

	return normalized
}
