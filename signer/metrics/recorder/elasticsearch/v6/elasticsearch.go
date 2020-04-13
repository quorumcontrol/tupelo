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
	"github.com/quorumcontrol/tupelo/sdk/gossip/types"
	"github.com/quorumcontrol/tupelo/signer/metrics/result"
)

type esResult struct {
	*result.Result
	DocType   string
	IndexedAt time.Time
}

type esTypeSummary struct {
	*result.Summary
	DocType   string
	IndexedAt time.Time
}

type allSummary struct {
	*result.Summary
	TypeCount map[string]uint64
	DocType   string
	IndexedAt time.Time
}

type Recorder struct {
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
	toStore := &esResult{result, "result", time.Now().UTC()}

	// elasticsearch fields must be of a consistent type, so just append
	// the golang type to the field name to prevent conflicts
	newTags := make(map[string]interface{})
	for k, v := range result.Tags {
		typeString := fmt.Sprintf("%T", v)
		newTags[k] = map[string]interface{}{typeString: v}
	}
	toStore.Tags = newTags

	docID := fmt.Sprintf("r%d_%s", result.Round, strings.ReplaceAll(result.Did, ":", "_"))

	return r.indexDoc(ctx, docID, toStore)
}

func (r *Recorder) Finish(ctx context.Context, summaries []*result.Summary) error {
	indexedAt := time.Now().UTC()

	var lastRoundCid string
	if r.startRound.Height() > 0 {
		lastRoundCid = r.startRound.CID().String()
	}

	all := &allSummary{
		Summary: &result.Summary{
			Type:         "_all",
			Round:        r.round.Height(),
			RoundCid:     r.round.CID().String(),
			LastRound:    r.startRound.Height(),
			LastRoundCid: lastRoundCid,
		},
		DocType:   "summary",
		TypeCount: make(map[string]uint64),
		IndexedAt: indexedAt,
	}

	for _, summary := range summaries {
		all.TypeCount[summary.Type] = summary.Count
		all.TotalBlocks += summary.TotalBlocks
		all.DeltaBlocks += summary.DeltaBlocks
		all.Count += summary.Count
		all.DeltaCount += summary.DeltaCount

		doc := &esTypeSummary{summary, "typesummary", indexedAt}
		typeDocID := fmt.Sprintf("r%d_%s", doc.Round, doc.Type)
		err := r.indexDoc(ctx, typeDocID, doc)

		if err != nil {
			return err
		}
	}

	docID := fmt.Sprintf("r%d", all.Round)
	err := r.indexDoc(ctx, docID, all)
	if err != nil {
		return err
	}

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
