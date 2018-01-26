package ownership

import (
	"github.com/quorumcontrol/qc3/did"
	"github.com/kazarena/json-gold/ld"
	"golang.org/x/crypto/ed25519"
	"github.com/fatih/structs"
	"fmt"
	"time"
	"strconv"
	"encoding/base64"
)

type Caveat struct {
	Type string `json:"string"`
	Uri string `json:"uri"`
}

type Proof struct {
	Type string `json:"type"`
	Created string `json:"created"`
	Creator string `json:"creator"`
	SignatureValue string `json:"signatureValue"`
}

type SignableCapability struct {
	Context string `json:"@context"`
	Id string `json:"id"`
	Type string `json:"type"`
	Target string `json:"target"`
	ParentCapability string `json:"parentCapability"`
	AuthenticationMaterial []string `json:"authenticationMaterial"`
	Caveat []*Caveat `json:"caveat"`
}

type Capability struct {
	SignableCapability
	Proof []*Proof `json:"proof"`
}

func (c *Capability) Sign(did did.Did, secretSigningKey ed25519.PrivateKey) error {
	proc := ld.NewJsonLdProcessor()
	options := ld.NewJsonLdOptions("")
	options.Format = "application/nquads"
	options.Algorithm = "URDNA2015"

	doc := structs.Map(c.SignableCapability)

	normalizedTriples, err := proc.Normalize(doc, options)
	if err != nil {
		return fmt.Errorf("error normalizing: %v", err)
	}

	sig := ed25519.Sign(secretSigningKey, []byte(normalizedTriples.(string)))

	c.Proof = append(c.Proof, &Proof{
		Type: "URDNA2015-ed25519",
		Created: strconv.Itoa(int(time.Now().UTC().Unix())),
		Creator: did.Id,
		SignatureValue: base64.StdEncoding.EncodeToString(sig),
	})
	return nil
}

