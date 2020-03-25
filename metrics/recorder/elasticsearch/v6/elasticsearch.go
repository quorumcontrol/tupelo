package elasticsearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	neturl "net/url"
	"strings"
	"time"

	"github.com/elastic/go-elasticsearch/v6"
	"github.com/elastic/go-elasticsearch/v6/esapi"
	"github.com/ipfs/go-cid"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip/types"
	"github.com/quorumcontrol/tupelo/metrics/result"
)

type esResult struct {
	*result.Result
	DocType   string
	IndexedAt time.Time
}

type byTypeAggregator struct {
	Count        uint64
	DocType      string
	Type         string
	Round        uint64
	RoundCid     string
	LastRound    uint64
	LastRoundCid string
	TotalBlocks  uint64
	DeltaBlocks  uint64
	IndexedAt    time.Time
}

type summary struct {
	Count        uint64
	DocType      string
	TypeCount    map[string]uint64
	Round        uint64
	RoundCid     string
	LastRound    uint64
	LastRoundCid string
	TotalBlocks  uint64
	DeltaBlocks  uint64
	IndexedAt    time.Time
}

type Recorder struct {
	byType       map[string]*byTypeAggregator
	round        *types.RoundWrapper
	startRound   *types.RoundWrapper
	client       *elasticsearch.Client
	index        string
	shardIndices bool
}

type roundPointer struct {
	Round     uint64
	RoundCid  string
	DocType   string
	IndexedAt time.Time
}

const roundsPerIndex = 1000

func New(url string, index string) (*Recorder, error) {
	esURL, err := neturl.Parse(url)
	if err != nil {
		return nil, err
	}

	var client *elasticsearch.Client

	if strings.HasSuffix(esURL.Hostname(), "es.amazonaws.com") {
		client, err = NewAwsEsClient(url)
	} else {
		client, err = elasticsearch.NewClient(elasticsearch.Config{
			Addresses: []string{url},
		})
	}
	if err != nil {
		return nil, err
	}

	return NewWithClient(client, index), nil
}

func NewWithClient(client *elasticsearch.Client, index string) *Recorder {
	r := &Recorder{
		client: client,
		index:  index,
		byType: make(map[string]*byTypeAggregator),
	}

	if r.index == "" {
		r.index = "tupelo-net-metrics"
		r.shardIndices = true
	}

	return r
}

func (r *Recorder) Start(ctx context.Context, startRound *types.RoundWrapper, endRound *types.RoundWrapper) error {
	r.round = endRound
	r.startRound = startRound
	return nil
}

func (r *Recorder) Record(ctx context.Context, result *result.Result) error {
	if _, ok := r.byType[result.Type]; !ok {
		r.byType[result.Type] = &byTypeAggregator{Type: result.Type}
	}

	r.byType[result.Type].DeltaBlocks += result.DeltaBlocks
	r.byType[result.Type].TotalBlocks += result.TotalBlocks
	r.byType[result.Type].Count++

	docID := fmt.Sprintf("r%d_%s", result.Round, strings.ReplaceAll(result.Did, ":", "_"))

	return r.indexDoc(ctx, docID, &esResult{result, "result", time.Now().UTC()})
}

func (r *Recorder) Finish(ctx context.Context) error {
	indexedAt := time.Now().UTC()

	var lastRoundCid string
	if r.startRound.Height() > 0 {
		lastRoundCid = r.startRound.CID().String()
	}

	summary := &summary{
		DocType:      "summary",
		Round:        r.round.Height(),
		RoundCid:     r.round.CID().String(),
		LastRound:    r.startRound.Height(),
		LastRoundCid: lastRoundCid,
		TypeCount:    make(map[string]uint64),
		IndexedAt:    indexedAt,
	}

	for _, result := range r.byType {
		summary.TypeCount[result.Type] = result.Count
		summary.TotalBlocks += result.TotalBlocks
		summary.DeltaBlocks += result.DeltaBlocks
		summary.Count += result.Count

		result.Round = r.round.Height()
		result.LastRound = r.startRound.Height()
		result.LastRoundCid = lastRoundCid
		result.DocType = "typesummary"
		result.RoundCid = r.round.CID().String()
		result.IndexedAt = indexedAt

		typeDocID := fmt.Sprintf("r%d_%s", result.Round, result.Type)
		err := r.indexDoc(ctx, typeDocID, result)

		if err != nil {
			return err
		}
	}

	docID := fmt.Sprintf("r%d", summary.Round)
	err := r.indexDoc(ctx, docID, summary)
	if err != nil {
		return err
	}

	// just so we can use the indexDoc function
	origShard := r.shardIndices
	defer func() { r.shardIndices = origShard }()
	r.shardIndices = false

	return r.indexDocToIndex(ctx, r.index, "roundpointer", &roundPointer{
		DocType:   "roundpointer",
		Round:     r.round.Height(),
		RoundCid:  r.round.CID().String(),
		IndexedAt: indexedAt,
	})
}

func (r *Recorder) LastRecordedRound(ctx context.Context) (*cid.Cid, error) {
	req := esapi.GetSourceRequest{
		Index:      r.index,
		DocumentID: "roundpointer",
	}

	res, err := req.Do(ctx, r.client)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.IsError() {
		// Does not exist yet
		if res.StatusCode == 404 {
			return nil, nil
		}
		return nil, fmt.Errorf(res.Status())
	}

	last := &roundPointer{}

	err = json.NewDecoder(res.Body).Decode(&last)
	if err != nil {
		return nil, err
	}

	lastCid, err := cid.Parse(last.RoundCid)
	if err != nil {
		return nil, err
	}

	return &lastCid, nil
}

func (r *Recorder) IndexForRound(round uint64) string {
	if !r.shardIndices {
		return r.index
	}
	return fmt.Sprintf("%s-%010d", r.index, int64(math.Ceil(float64(r.round.Height())/float64(roundsPerIndex))*roundsPerIndex))
}

func (r *Recorder) indexDoc(ctx context.Context, id string, result interface{}) error {
	index := r.IndexForRound(r.round.Height())
	return r.indexDocToIndex(ctx, index, id, result)
}

func (r *Recorder) indexDocToIndex(ctx context.Context, index string, id string, result interface{}) error {
	doc, err := json.Marshal(result)
	if err != nil {
		return err
	}

	req := esapi.IndexRequest{
		Index:      index,
		DocumentID: id,
		Body:       bytes.NewBuffer(doc),
	}

	res, err := req.Do(ctx, r.client)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.IsError() {
		return fmt.Errorf(res.Status())
	}
	return nil
}
