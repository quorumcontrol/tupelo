package gossip2

import "github.com/ethereum/go-ethereum/crypto"

// ID in storage is 32bitsConflictSetId|32bitssignaturehash|transactionid|signaturehash|"-signature" OR signature before transactionid or before signaturehash
func (s *Signature) StoredID(conflictSetID []byte) []byte {
	encodedSig, err := s.MarshalMsg(nil)
	if err != nil {
		panic("Could not marshal signature")
	}
	id := crypto.Keccak256(encodedSig)
	return concatBytesSlice(conflictSetID[0:4], id[0:4], []byte{byte(MessageTypeSignature)}, s.TransactionID, id, conflictSetID)
}

// [254 216 115 227 ///// 35 11 251 183 //// 0 //// 57 79 66 166 142 30 188 4 223 174 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0]

//signatureID conflictSet[0-3],signatureHash[4-7],sigantureByte[8],transactionId[9-41],fullSignatureId[42-74]

func transactionIDFromSignatureKey(key []byte) []byte {
	return concatBytesSlice(key[0:4], key[9:13], []byte{byte(MessageTypeTransaction)}, key[9:41], key[73:105])
}
