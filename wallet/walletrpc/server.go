package walletrpc

import (
	"net"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/qc3/consensus"
	"golang.org/x/net/context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

const (
	port = ":50051"
)

type server struct {
	NotaryGroup *consensus.Group
}

func (s *server) GenerateKey(ctx context.Context, req *GenerateKeyRequest) (*GenerateKeyResponse, error) {
	session := NewSession(req.Creds, s.NotaryGroup)

	key, err := session.GenerateKey()
	if err != nil {
		return nil, err
	}

	addr := crypto.PubkeyToAddress(key.PublicKey)
	return &GenerateKeyResponse{
		KeyAddr: addr.String(),
	}, nil
}

func (s *server) ListKeys(ctx context.Context, req *ListKeysRequest) (*ListKeysResponse, error) {
	session := NewSession(req.Creds, s.NotaryGroup)

	keys, err := session.ListKeys()
	if err != nil {
		return nil, err
	}

	return &ListKeysResponse{
		KeyAddrs: keys,
	}, nil
}

func (s *server) CreateChain(ctx context.Context, req *GenerateChainRequest) (*GenerateChainResponse, error) {
	session := NewSession(req.Creds, s.NotaryGroup)

	chain, err := session.CreateChain(req.KeyAddr)
	if err != nil {
		return nil, err
	}

	id, err := chain.Id()
	if err != nil {
		return nil, err
	}

	return &GenerateChainResponse{
		ChainId: id,
	}, nil
}

func (s *server) ListChainIds(ctx context.Context, req *ListChainIdsRequest) (*ListChainIdsResponse, error) {
	session := NewSession(req.Creds, s.NotaryGroup)

	ids, err := session.GetChainIds()
	if err != nil {
		return nil, err
	}

	return &ListChainIdsResponse{
		ChainIds: ids,
	}, nil
}

func (s *server) GetTip(ctx context.Context, req *GetTipRequest) (*GetTipResponse, error) {
	session := NewSession(req.Creds, s.NotaryGroup)

	tipCid, err := session.GetTip(req.ChainId)
	if err != nil {
		return nil, err
	}

	return &GetTipResponse{
		Tip: tipCid.String(),
	}, nil
}

// func (s *server) PlayTransactions(ctx context.Context, req *PlayTransactionsRequest) PlayTransactionsResponse {
//	session := NewSession(req.creds, s.NotaryGroup)
// }

func Serve(group *consensus.Group) (*grpc.Server, error) {
	listener, err := net.Listen("tcp", port)
	if err != nil {
		return nil, err
	}

	grpcServer := grpc.NewServer()
	s := &server{
		NotaryGroup: group,
	}

	RegisterWalletRPCServiceServer(grpcServer, s)
	reflection.Register(grpcServer)

	if err := grpcServer.Serve(listener); err != nil {
		return nil, err
	}

	return grpcServer, nil
}
