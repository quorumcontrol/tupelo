package validators

import (
	"github.com/quorumcontrol/qc3/did"
	"strings"
	"fmt"
	"github.com/quorumcontrol/qc3/storage"
	"github.com/quorumcontrol/qc3/ownership"
	log "github.com/sirupsen/logrus"
)

func IsValidCapability(storage *storage.Storage, cap ownership.Capability) (bool,error) {
	if len(cap.Proof) == 0 {
		return false, nil
	}

	dids := make(map[string]did.Did)

	for _,key := range cap.AuthenticationMaterial {
		didAndKey := strings.Split(key.Id, "#")
		did,err := storage.GetDid(didAndKey[0])
		if err != nil {
			return false, fmt.Errorf("erorr getting did from key material %v", err)
		}
		if did != nil {
			dids[did.Id] = *did
		} else {
			return false, fmt.Errorf("unkown did %s", didAndKey[0])
		}
	}

	validProofs := make([]ownership.Proof, 0)

	for _,proof := range cap.Proof {
		creatorDidAndKey := strings.Split(proof.Creator, "#")
		creator,ok := dids[creatorDidAndKey[0]]

		if ok {
			isValid,err := cap.VerifyProof(*proof, creator)
			if err != nil {
				log.WithFields(log.Fields{
					"cap": cap.Id,
					"did": creator.Id,
					"error": err,
				}).Errorf("error, invalid proof")
				return false, fmt.Errorf("error verifying proof from %s: %v", proof.Creator, err)
			}
			if isValid {
				validProofs = append(validProofs, *proof)
			} else {
				log.WithFields(log.Fields{
					"cap": cap.Id,
					"did": creator.Id,
				}).Errorf("error, invalid proof")
			}
		}
	}

	if len(validProofs) == 0 {
		log.WithFields(log.Fields{
			"cap": cap.Id,
			"error": "no valid proofs",
		}).Errorf("no valid proofs")
		return false, nil
	}

	if cap.ParentCapability != "" {
		// TODO: parent stuff
	}

	//TODO: handle the caveats

	return true,nil
}
