package validators

import (
	"github.com/quorumcontrol/qc3/storage"
	"github.com/quorumcontrol/qc3/did"
	"fmt"
	log "github.com/sirupsen/logrus"
)


func ValidateDidInsert(storage *storage.Storage, did did.Did) (bool,error) {
	existing,err := storage.GetDid(did.Id)
	if err != nil {
		return false, fmt.Errorf("error getting did: %v", err)
	}
	if existing != nil {
		log.WithFields(log.Fields{
			"error": "duplicate did attempt",
			"did": did.Id,
		}).Errorf("error, duplicate did attempt")
		return false, nil
	}

	return true, nil
}