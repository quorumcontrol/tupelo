package storage

import (
	"github.com/attic-labs/noms/go/datas"
	"github.com/attic-labs/noms/go/types"
	"github.com/quorumcontrol/qc3/did"
	"github.com/attic-labs/noms/go/marshal"
	"fmt"
	"github.com/quorumcontrol/qc3/ownership"
)

const didDocsDatasetId = "didDocs"
const capabilitiesDatasetId = "capabilities"

type Storage struct {
	Db datas.Database
}

func (s *Storage) datasetMap(name string) (types.Map) {
	s.Db.Rebase()
	ds := s.Db.GetDataset(name)
	m, ok := ds.MaybeHeadValue()
	if ok {
		return m.(types.Map)
	} else {
		return types.NewMap(s.Db)
	}
}

func (s *Storage) GetDidDocs() (types.Map) {
  return s.datasetMap(didDocsDatasetId)
}

func (s *Storage) GetCapabilities() (types.Map) {
	return s.datasetMap(capabilitiesDatasetId)
}

func (s *Storage) UpsertDid(didDoc did.Did) error {
	marshaled,err := marshal.Marshal(s.Db, didDoc)
	if err != nil {
		return fmt.Errorf("error marshaling: %v", err)
	}

	didDocs := s.GetDidDocs().Edit().Set(types.String(didDoc.Id), marshaled).Map()

	_,err = s.Db.CommitValue(s.Db.GetDataset(didDocsDatasetId), didDocs)
	if err != nil {
		return fmt.Errorf("error committing: %v", err)
	}
	return nil
}

func (s *Storage) GetDid(id string) (*did.Did, error) {
	marshaled,ok := s.GetDidDocs().MaybeGet(types.String(id))
	if !ok {
		return nil, nil
	}

	didDoc := &did.Did{}
	err := marshal.Unmarshal(marshaled, didDoc)
	if err != nil {
		return nil, fmt.Errorf("error unmarshaling: %v", err)
	}

	return didDoc,nil
}

func (s *Storage) UpsertCapability(capability ownership.Capability) error {
	marshaled,err := marshal.Marshal(s.Db, capability)
	if err != nil {
		return fmt.Errorf("error marshaling: %v", err)
	}

	capabilities := s.GetCapabilities().Edit().Set(types.String(capability.Id), marshaled).Map()

	_,err = s.Db.CommitValue(s.Db.GetDataset(capabilitiesDatasetId), capabilities)
	if err != nil {
		return fmt.Errorf("error committing: %v", err)
	}

	return nil
}

func (s *Storage) GetCapability(id string) (*ownership.Capability, error) {
	marshaled,ok := s.GetCapabilities().MaybeGet(types.String(id))
	if !ok {
		return nil, nil
	}

	capability := &ownership.Capability{}
	err := marshal.Unmarshal(marshaled, capability)
	if err != nil {
		return nil, fmt.Errorf("error unmarshaling: %v", err)
	}

	return capability,nil
}

