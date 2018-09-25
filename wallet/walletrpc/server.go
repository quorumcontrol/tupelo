package walletrpc

import (
	"fmt"
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
	defer session.Stop()

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
	defer session.Stop()

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
	defer session.Stop()

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
	defer session.Stop()

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
	defer session.Stop()

	tipCid, err := session.GetTip(req.ChainId)
	if err != nil {
		return nil, err
	}

	return &GetTipResponse{
		Tip: tipCid.String(),
	}, nil
}

func (s *server) SetOwner(ctx context.Context, req *SetOwnerRequest) (*SetOwnerResponse, error) {
	session := NewSession(req.Creds, s.NotaryGroup)
	defer session.Stop()

	newTip, err := session.SetOwner(req.ChainId, req.KeyAddr, req.NewOwnerKeys)
	if err != nil {
		return nil, err
	}

	return &SetOwnerResponse{
		Tip: newTip.String(),
	}, nil
}

func (s *server) SetData(ctx context.Context, req *SetDataRequest) (*SetDataResponse, error) {
	session := NewSession(req.Creds, s.NotaryGroup)
	defer session.Stop()

	tipCid, err := session.SetData(req.ChainId, req.KeyAddr, req.Path, req.Value)
	if err != nil {
		return nil, err
	}

	return &SetDataResponse{
		Tip: tipCid.String(),
	}, nil
}

func (s *server) EstablishCoin(ctx context.Context, req *EstablishCoinRequest) (*EstablishCoinResponse, error) {
	session := NewSession(req.Creds, s.NotaryGroup)
	defer session.Stop()

	tipCid, err := session.EstablishCoin(req.ChainId, req.KeyAddr, req.CoinName, req.Maximum)
	if err != nil {
		return nil, err
	}

	return &EstablishCoinResponse{
		Tip: tipCid.String(),
	}, nil
}

func (s *server) MintCoin(ctx context.Context, req *MintCoinRequest) (*MintCoinResponse, error) {
	session := NewSession(req.Creds, s.NotaryGroup)
	defer session.Stop()

	tipCid, err := session.MintCoin(req.ChainId, req.KeyAddr, req.CoinName, req.Amount)
	if err != nil {
		return nil, err
	}

	return &MintCoinResponse{
		Tip: tipCid.String(),
	}, nil
}

func Serve(group *consensus.Group) (*grpc.Server, error) {
	fmt.Println("Starting Tupelo RPC server")

	fmt.Println("Listening on port", port)
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
