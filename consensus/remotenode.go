package consensus

type RemoteNode struct {
	VerKey PublicKey
	DstKey PublicKey
	Id     string
}

func NewRemoteNode(verKey, dstKey PublicKey) *RemoteNode {
	return &RemoteNode{
		Id:     PublicKeyToAddr(&verKey),
		VerKey: verKey,
		DstKey: dstKey,
	}
}
