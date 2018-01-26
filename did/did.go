package did

import (
	"golang.org/x/crypto/ed25519"
	"crypto/rand"
	"fmt"
	"golang.org/x/crypto/nacl/box"
	"github.com/btcsuite/btcutil/base58"
	"encoding/base64"
)

//{
//"id": "did:example:123456789abcdefghi/keys/2",
//"type": "Ed25519SigningKey",
//"owner": "did:example:123456789abcdefghi",
//"expires": "2017-02-08T16:02:20Z",
//"publicKeyBase64": "IOmA4R7TfhkYTYW87...CBMq2/gi25s="
//}

type PublicKey struct {
	Id string `json:"id"`
	Type string `json:"type"`
	Owner string `json:"owner"`
	PublicKeyPem string `json:"owner,omitempty",noms:",omitempty"`
	PublicKeyBase64 string `json:"publicKeyBase64,omitempty",noms:",omitempty"`
}

type Authentication struct {
	Type string `json:"type"`
	PublicKey string `json:"publicKey"`
}

type Service struct {
	Type string `json:"type"`
	ServiceEndpoint string `json:"serviceEndpoint"`
}

type Did struct {
	Context string `json:"@context"`
	Id string `json:"id"`
	PublicKey []PublicKey `json:"publicKey"`
	Authentication []Authentication `json:"authentication"`
	Service []Service `json:"service"`
}

type Secret struct {
	SecretSigningKey []byte
	SecretEncryptionKey []byte
}

func Generate() (*Did, *Secret, error) {
	signingPublic,signingPrivate,err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("error generating signing key: %v", err)
	}

	encryptionPublic, encryptionPrivate, err := box.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("error generating encryption key: %v", err)
	}

	id := "did:qc:" + base58.Encode(signingPublic[0:16])

	return &Did{
		Context: "https://w3id.org/did/v1",
		Id: id,
		PublicKey: []PublicKey{
			{
				Id: id + "#keySigning0",
				Type: "Ed25519SigningKey",
				Owner: id,
				PublicKeyBase64: base64.StdEncoding.EncodeToString(signingPublic),
			},
			{
				Id: id + "#keyEncryption0",
				Type: "NaclBoxEncryptionKey",
				Owner: id,
				PublicKeyBase64: base64.StdEncoding.EncodeToString(encryptionPublic[:]),
			},
		},
		Authentication: []Authentication{
			{
				Type: "Ed25519SignatureAuthentication",
				PublicKey: id + "#keySigning0",
			},
		},
	}, &Secret{
		SecretSigningKey: signingPrivate,
		SecretEncryptionKey: encryptionPrivate[:],
	}, nil
}

func (d Did) GetSigningKey() *PublicKey {
	for _,key := range d.PublicKey {
		if key.Type == "Ed25519SigningKey" {
			return &key
		}
	}
	return nil
}
