package gossip4

import "sync"

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

func (s *Snowball) Tick(votes []*Vote) {
	s.Lock()
	defer s.Unlock()

	if s.decided {
		return
	}

	votes = calculateTallies(votes)

	var majority *Vote

	for _, vote := range votes {
		// Empty vote can still have tally (the base tally), so we need to ignore empty vote.
		if vote.ID() == ZeroVoteID {
			continue
		}

		if majority == nil || vote.Tally() > majority.Tally() {
			majority = vote
		}
	}

	denom := float64(len(votes))

	if denom < 2 {
		denom = 2
	}

	if majority == nil || majority.Tally() < s.alpha*2/denom {
		s.count = 0
		return
	}

	s.counts[majority.ID()]++

	if s.preferred == nil || s.counts[majority.ID()] > s.counts[s.preferred.ID()] {
		s.preferred = majority
	}

	if s.last == nil || majority.ID() != s.last.ID() {
		s.last, s.count = majority, 1
	} else {
		s.count++

		if s.count > s.beta {
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
