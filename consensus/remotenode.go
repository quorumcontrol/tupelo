package consensus

type RemoteNode struct {
	VerKey PublicKey
	DstKey PublicKey
	Id     string
}

func NewRemoteNode(verKey, dstKey PublicKey) *RemoteNode {
	remoteNode := &RemoteNode{
		VerKey: verKey,
		DstKey: dstKey,
	}

	remoteNode.Id = PublicKeyToAddr(&verKey)
	return remoteNode
}
