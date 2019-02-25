package walletrpc

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/improbable-eng/grpc-web/go/grpcweb"
	cbornode "github.com/ipfs/go-ipld-cbor"
	"github.com/quorumcontrol/tupelo-go-client/consensus"
	gossip3client "github.com/quorumcontrol/tupelo-go-client/client"
	"github.com/quorumcontrol/tupelo/wallet"
	"github.com/quorumcontrol/tupelo/wallet/adapters"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
)

const (
	defaultPort    = ":50051"
	defaultWebPort = ":50050"
)

type server struct {
	Client      *gossip3client.Client
	storagePath string
}

func (s *server) Register(ctx context.Context, req *RegisterWalletRequest) (*RegisterWalletResponse, error) {
	walletName := req.Creds.WalletName
	session, err := NewSession(s.storagePath, walletName, s.Client)
	if err != nil {
		return nil, err
	}
	defer session.Stop()

	err = session.CreateWallet(req.Creds.PassPhrase)
	if err != nil {
		if _, ok := err.(*wallet.CreateExistingWalletError); ok {
			msg := fmt.Sprintf("wallet %v already exists", walletName)
			return nil, status.Error(codes.AlreadyExists, msg)
		}
		return nil, fmt.Errorf("error creating wallet: %v", err)
	}

	return &RegisterWalletResponse{
		WalletName: req.Creds.WalletName,
	}, nil
}

func (s *server) GenerateKey(ctx context.Context, req *GenerateKeyRequest) (*GenerateKeyResponse, error) {
	session, err := NewSession(s.storagePath, req.Creds.WalletName, s.Client)
	if err != nil {
		return nil, err
	}

	err = session.Start(req.Creds.PassPhrase)
	if err != nil {
		return nil, fmt.Errorf("error starting session: %v", err)
	}

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
	session, err := NewSession(s.storagePath, req.Creds.WalletName, s.Client)
	if err != nil {
		return nil, err
	}

	err = session.Start(req.Creds.PassPhrase)
	if err != nil {
		return nil, fmt.Errorf("error starting session: %v", err)
	}

	defer session.Stop()

	keys, err := session.ListKeys()
	if err != nil {
		return nil, err
	}

	return &ListKeysResponse{
		KeyAddrs: keys,
	}, nil
}

func (s *server) parseStorageAdapter(c *StorageAdapterConfig) (*adapters.Config, error) {
	// Default adapters.Config is set in session.go
	if c == nil {
		return nil, nil
	}

	switch config := c.AdapterConfig.(type) {
	case *StorageAdapterConfig_Badger:
		return &adapters.Config{
			Adapter: adapters.BadgerStorageAdapterName,
			Arguments: map[string]interface{}{
				"path": config.Badger.Path,
			},
		}, nil
	case *StorageAdapterConfig_Ipld:
		return &adapters.Config{
			Adapter: adapters.IpldStorageAdapterName,
			Arguments: map[string]interface{}{
				"path":    config.Ipld.Path,
				"address": config.Ipld.Address,
				"online":  !config.Ipld.Offline,
			},
		}, nil
	default:
		return nil, fmt.Errorf("Unsupported storage adapter sepcified")
	}
}

func (s *server) CreateChainTree(ctx context.Context, req *GenerateChainRequest) (*GenerateChainResponse, error) {
	session, err := NewSession(s.storagePath, req.Creds.WalletName, s.Client)
	if err != nil {
		return nil, err
	}

	err = session.Start(req.Creds.PassPhrase)
	if err != nil {
		return nil, fmt.Errorf("error starting session: %v", err)
	}

	defer session.Stop()

	storageAdapterConfig, err := s.parseStorageAdapter(req.StorageAdapter)
	if err != nil {
		return nil, err
	}

	chain, err := session.CreateChain(req.KeyAddr, storageAdapterConfig)
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

func (s *server) ExportChainTree(ctx context.Context, req *ExportChainRequest) (*ExportChainResponse, error) {
	session, err := NewSession(s.storagePath, req.Creds.WalletName, s.Client)
	if err != nil {
		return nil, err
	}

	err = session.Start(req.Creds.PassPhrase)
	if err != nil {
		return nil, fmt.Errorf("error starting session: %v", err)
	}

	defer session.Stop()

	serializedChain, err := session.ExportChain(req.ChainId)
	if err != nil {
		return nil, err
	}

	return &ExportChainResponse{
		ChainTree: serializedChain,
	}, nil
}

func (s *server) ImportChainTree(ctx context.Context, req *ImportChainRequest) (*ImportChainResponse, error) {
	session, err := NewSession(s.storagePath, req.Creds.WalletName, s.Client)
	if err != nil {
		return nil, err
	}

	err = session.Start(req.Creds.PassPhrase)
	if err != nil {
		return nil, fmt.Errorf("error starting session: %v", err)
	}

	defer session.Stop()

	storageAdapterConfig, err := s.parseStorageAdapter(req.StorageAdapter)
	if err != nil {
		return nil, err
	}

	chain, err := session.ImportChain(req.ChainTree, storageAdapterConfig)

	if err != nil {
		return nil, err
	}

	chainId, err := chain.Id()
	if err != nil {
		return nil, err
	}

	return &ImportChainResponse{
		ChainId: chainId,
	}, nil
}

func (s *server) ListChainIds(ctx context.Context, req *ListChainIdsRequest) (*ListChainIdsResponse, error) {
	session, err := NewSession(s.storagePath, req.Creds.WalletName, s.Client)
	if err != nil {
		return nil, err
	}

	err = session.Start(req.Creds.PassPhrase)
	if err != nil {
		return nil, fmt.Errorf("error starting session: %v", err)
	}

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
	session, err := NewSession(s.storagePath, req.Creds.WalletName, s.Client)
	if err != nil {
		return nil, err
	}

	err = session.Start(req.Creds.PassPhrase)
	if err != nil {
		return nil, fmt.Errorf("error starting session: %v", err)
	}

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
	session, err := NewSession(s.storagePath, req.Creds.WalletName, s.Client)
	if err != nil {
		return nil, err
	}

	err = session.Start(req.Creds.PassPhrase)
	if err != nil {
		return nil, fmt.Errorf("error starting session: %v", err)
	}

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
	session, err := NewSession(s.storagePath, req.Creds.WalletName, s.Client)
	if err != nil {
		return nil, err
	}

	err = session.Start(req.Creds.PassPhrase)
	if err != nil {
		return nil, fmt.Errorf("error starting session: %v", err)
	}

	defer session.Stop()

	tipCid, err := session.SetData(req.ChainId, req.KeyAddr, req.Path, req.Value)
	if err != nil {
		return nil, err
	}

	return &SetDataResponse{
		Tip: tipCid.String(),
	}, nil
}

func (s *server) Resolve(ctx context.Context, req *ResolveRequest) (*ResolveResponse, error) {
	session, err := NewSession(s.storagePath, req.Creds.WalletName, s.Client)
	if err != nil {
		return nil, err
	}

	err = session.Start(req.Creds.PassPhrase)
	if err != nil {
		return nil, fmt.Errorf("error starting session: %v", err)
	}

	defer session.Stop()

	pathSegments, err := consensus.DecodePath(req.Path)
	if err != nil {
		return nil, fmt.Errorf("bad path: %v", err)
	}

	data, remainingSegments, err := session.Resolve(req.ChainId, pathSegments)
	if err != nil {
		return nil, err
	}

	remainingPath := strings.Join(remainingSegments, "/")
	dataBytes, err := cbornode.DumpObject(data)
	if err != nil {
		return nil, err
	}

	return &ResolveResponse{
		RemainingPath: remainingPath,
		Data:          dataBytes,
	}, nil
}

func (s *server) EstablishCoin(ctx context.Context, req *EstablishCoinRequest) (*EstablishCoinResponse, error) {
	session, err := NewSession(s.storagePath, req.Creds.WalletName, s.Client)
	if err != nil {
		return nil, err
	}

	err = session.Start(req.Creds.PassPhrase)
	if err != nil {
		return nil, fmt.Errorf("error starting session: %v", err)
	}

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
	session, err := NewSession(s.storagePath, req.Creds.WalletName, s.Client)
	if err != nil {
		return nil, err
	}

	err = session.Start(req.Creds.PassPhrase)
	if err != nil {
		return nil, fmt.Errorf("error starting session: %v", err)
	}

	defer session.Stop()

	tipCid, err := session.MintCoin(req.ChainId, req.KeyAddr, req.CoinName, req.Amount)
	if err != nil {
		return nil, err
	}

	return &MintCoinResponse{
		Tip: tipCid.String(),
	}, nil
}

func startServer(grpcServer *grpc.Server, storagePath string, client *gossip3client.Client) (*grpc.Server, error) {
	fmt.Println("Starting Tupelo RPC server")

	listener, err := net.Listen("tcp", defaultPort)
	if err != nil {
		return nil, err
	}

	s := &server{
		Client:      client,
		storagePath: storagePath,
	}

	RegisterWalletRPCServiceServer(grpcServer, s)
	reflection.Register(grpcServer)

	go func() {
		fmt.Println("Listening on port", defaultPort)
		err := grpcServer.Serve(listener)
		if err != nil {
			log.Printf("error serving: %v", err)
		}
	}()
	return grpcServer, nil
}

func ServeInsecure(storagePath string, client *gossip3client.Client) (*grpc.Server, error) {
	grpcServer, err := startServer(grpc.NewServer(), storagePath, client)
	if err != nil {
		return nil, fmt.Errorf("error starting: %v", err)
	}
	return grpcServer, nil
}

func ServeTLS(storagePath string, client *gossip3client.Client, certFile string, keyFile string) (*grpc.Server, error) {
	creds, err := credentials.NewServerTLSFromFile(certFile, keyFile)
	if err != nil {
		return nil, err
	}

	credsOption := grpc.Creds(creds)
	grpcServer := grpc.NewServer(credsOption)

	return startServer(grpcServer, storagePath, client)
}

func ServeWebInsecure(grpcServer *grpc.Server) (*http.Server, error) {
	s, err := createGrpcWeb(grpcServer, defaultWebPort)
	if err != nil {
		return nil, fmt.Errorf("error creating GRPC server: %v", err)
	}
	go func() {
		fmt.Println("grpc-web listening on port", defaultWebPort)
		err := s.ListenAndServe()
		if err != nil {
			log.Printf("error listening: %v", err)
		}
	}()
	return s, nil
}

func ServeWebTLS(grpcServer *grpc.Server, certFile string, keyFile string) (*http.Server, error) {
	s, err := createGrpcWeb(grpcServer, defaultWebPort)
	if err != nil {
		return nil, fmt.Errorf("error creating GRPC server: %v", err)
	}
	go func() {
		fmt.Println("grpc-web listening on port", defaultWebPort)
		err := s.ListenAndServeTLS(certFile, keyFile)
		if err != nil {
			log.Printf("error listening: %v", err)
		}
	}()
	return s, nil
}

func createGrpcWeb(grpcServer *grpc.Server, port string) (*http.Server, error) {
	wrappedGrpc := grpcweb.WrapServer(grpcServer)
	handler := http.HandlerFunc(func(resp http.ResponseWriter, req *http.Request) {
		if wrappedGrpc.IsGrpcWebRequest(req) {
			wrappedGrpc.ServeHTTP(resp, req)
			return
		}

		if req.Method == "OPTIONS" {
			headers := resp.Header()
			headers.Add("Access-Control-Allow-Origin", "*")
			headers.Add("Access-Control-Allow-Headers", "*")
			headers.Add("Access-Control-Allow-Methods", "GET, POST,OPTIONS")
			resp.WriteHeader(http.StatusOK)
		} else {
			//TODO: this is a good place to stick in the UI
			log.Printf("unkown route: %v", req)
		}

	})
	s := &http.Server{
		Addr:    port,
		Handler: handler,
	}
	return s, nil
}
