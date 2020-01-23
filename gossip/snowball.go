package gossip

import (
	"context"
	"sync"

	logging "github.com/ipfs/go-log"
	"github.com/opentracing/opentracing-go"
)

var snowlog = logging.Logger("snowball")

type Snowball struct {
	sync.RWMutex

	alpha float64
	beta  int
	k     int

	count  int
	counts map[string]uint16

	preferred *Vote
	last      *Vote

	decided bool
}

func NewSnowball(alpha float64, beta int, k int) *Snowball {
	return &Snowball{
		counts: make(map[string]uint16),
		alpha:  alpha,
		beta:   beta,
		k:      k,
	}
}

func (s *Snowball) Reset() {
	s.Lock()

	s.preferred = nil
	s.last = nil

	s.counts = make(map[string]uint16)
	s.count = 0

	s.decided = false
	s.Unlock()
}

func (s *Snowball) Tick(startCtx context.Context, votes []*Vote) {
	sp, _ := opentracing.StartSpanFromContext(startCtx, "gossip4.snowball.tick")
	defer sp.Finish()

	s.Lock()
	defer s.Unlock()
	sp.LogKV("voteCount", len(votes))
	snowlog.Debugf("tick len(votes): %d", len(votes))

	if s.decided {
		return
	}

	var majority *Vote

	for _, vote := range votes {
		// Empty vote can still have tally (the base tally), so we need to ignore empty vote.
		if vote.ID() == ZeroVoteID {
			continue
		}

		snowlog.Debugf("id: %s", vote.ID())

		if majority == nil || vote.Tally() > majority.Tally() {
			majority = vote
		}
	}

	if majority != nil {
		sp.LogKV("majority", majority.ID())
	}

	denom := float64(len(votes))

	if denom < 2 {
		denom = 2
	}

	if majority == nil || majority.Tally() < s.alpha*2/denom {
		snowlog.Debugf("resettitng count: %v", majority)
		sp.LogKV("countReset", true)
		s.count = 0
		return
	}
	sp.LogKV("majorityTally", majority.Tally())
	snowlog.Debugf("majority %s tally: %f", majority.ID(), majority.Tally())

	s.counts[majority.ID()]++

	if s.preferred == nil || s.counts[majority.ID()] > s.counts[s.preferred.ID()] {
		sp.LogKV("setPreferred", true)
		snowlog.Debugf("setting preferred: %s", majority.ID())
		s.preferred = majority
	}

	if s.last == nil || majority.ID() != s.last.ID() {
		s.last, s.count = majority, 1
	} else {
		s.count++
		sp.LogKV("count", s.count)
		if s.count > s.beta {
			sp.LogKV("decided", true)
			s.decided = true
		}
	}
}

func (s *Snowball) Prefer(b *Vote) {
	s.Lock()
	s.preferred = b
	_, exists := s.counts[b.ID()]
	if !exists {
		s.counts[b.ID()] = 1
	}
	s.Unlock()
}

func (s *Snowball) Preferred() *Vote {
	s.RLock()
	preferred := s.preferred
	s.RUnlock()

	return preferred
}

func (s *Snowball) Decided() bool {
	s.RLock()
	decided := s.decided
	s.RUnlock()

	return decided
}

func (s *Snowball) Progress() int {
	s.RLock()
	progress := s.count
	s.RUnlock()

	return progress
}
