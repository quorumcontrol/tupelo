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

	totalCount uint64

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

	s.totalCount++

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
		// handle the special case of deciding on nil
		// where we will up our beta on getting nil
		if s.preferred == nil && majority == nil {
			s.IncAndCheckCount()
			return
		}

		// if we have a nil preference, then go ahead and prefer the highest tally no matter what
		if s.preferred == nil && majority != nil {
			s.PreferInLock(majority)
		}

		snowlog.Debugf("resetting count: %v", majority)
		sp.LogKV("countReset", true)
		s.count = 0
		return
	}
	sp.LogKV("majorityTally", majority.Tally())

	s.counts[majority.ID()]++

	if preferred := s.preferred; preferred != nil {
		snowlog.Debugf("majority %s tally: %f, count %d, snowballCount: %d (preferred: %s)", majority.ID(), majority.Tally(), s.counts[majority.ID()], s.count, s.preferred.Checkpoint.CID())
	} else {
		snowlog.Debugf("majority %s tally: %f, count %d, snowballCount: %d (preferred: nil)", majority.ID(), majority.Tally(), s.counts[majority.ID()], s.count)
	}

	if s.preferred == nil || s.counts[majority.ID()] > s.counts[s.preferred.ID()] {
		sp.LogKV("setPreferred", true)
		snowlog.Debugf("setting preferred: %s @ %d", majority.ID(), s.count)
		s.preferred = majority
	}

	if last := s.last; last == nil || majority.ID() != s.last.ID() {
		var lastID string
		if last != nil {
			lastID = last.ID()
		}
		snowlog.Debugf("majority id: %s != last id: %s", majority.ID(), lastID)
		s.last, s.count = majority, 1
	} else {
		if decided := s.IncAndCheckCount(); decided {
			sp.LogKV("decided", true)
		}
		sp.LogKV("count", s.count)
	}
}

func (s *Snowball) IncAndCheckCount() bool {
	s.count++
	snowlog.Debugf("count: %d", s.count)
	if s.count > s.beta {
		snowlog.Debugf("decided")
		s.decided = true
	}
	return s.decided
}

func (s *Snowball) Prefer(v *Vote) {
	s.Lock()
	s.PreferInLock(v)
	s.Unlock()
}

// PreferInLock is ONLY for contexts in which you already hold a write lock
func (s *Snowball) PreferInLock(v *Vote) {
	s.preferred = v
	_, exists := s.counts[v.ID()]
	if !exists {
		s.counts[v.ID()] = 1
	}
}

func (s *Snowball) Preferred() *Vote {
	s.RLock()
	defer s.RUnlock()
	if preferred := s.preferred; preferred == nil {
		return nil
	}

	return s.preferred.Copy()
}

func (s *Snowball) Decided() bool {
	s.RLock()
	defer s.RUnlock()

	return s.decided
}

func (s *Snowball) Progress() int {
	s.RLock()
	progress := s.count
	s.RUnlock()

	return progress
}
