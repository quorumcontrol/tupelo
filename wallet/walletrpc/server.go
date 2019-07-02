package walletrpc

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/improbable-eng/grpc-web/go/grpcweb"
	cid "github.com/ipfs/go-cid"
	cbornode "github.com/ipfs/go-ipld-cbor"
	"github.com/quorumcontrol/messages/build/go/services"
	"github.com/quorumcontrol/messages/build/go/transactions"
	"github.com/quorumcontrol/tupelo-go-sdk/consensus"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/remote"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/types"
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
	defaultWebPort = ":50050"
)

type server struct {
	NotaryGroup *types.NotaryGroup
	PubSub      remote.PubSub
	storagePath string
}

func (s *server) Register(ctx context.Context, req *services.RegisterWalletRequest) (*services.RegisterWalletResponse, error) {
	walletName := req.Creds.WalletName
	session, err := NewSession(s.storagePath, walletName, s.NotaryGroup, s.PubSub)
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

	return &services.RegisterWalletResponse{
		WalletName: req.Creds.WalletName,
	}, nil
}

func (s *server) GenerateKey(ctx context.Context, req *services.GenerateKeyRequest) (*services.GenerateKeyResponse, error) {
	session, err := NewSession(s.storagePath, req.Creds.WalletName, s.NotaryGroup, s.PubSub)
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
	return &services.GenerateKeyResponse{
		KeyAddr: addr.String(),
	}, nil
}

func (s *server) ListKeys(ctx context.Context, req *services.ListKeysRequest) (*services.ListKeysResponse, error) {
	session, err := NewSession(s.storagePath, req.Creds.WalletName, s.NotaryGroup, s.PubSub)
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

	return &services.ListKeysResponse{
		KeyAddrs: keys,
	}, nil
}

func (s *server) parseStorageAdapter(c *services.StorageAdapterConfig) (*adapters.Config, error) {
	// Default adapters.Config is set in session.go
	if c == nil {
		return nil, nil
	}

	switch config := c.AdapterConfig.(type) {
	case *services.StorageAdapterConfig_Badger:
		return &adapters.Config{
			Adapter: adapters.BadgerStorageAdapterName,
			Arguments: map[string]interface{}{
				"path": config.Badger.Path,
			},
		}, nil
	default:
		return nil, fmt.Errorf("Unsupported storage adapter specified")
	}
}

func (s *server) CreateChainTree(ctx context.Context, req *services.GenerateChainRequest) (*services.GenerateChainResponse, error) {
	session, err := NewSession(s.storagePath, req.Creds.WalletName, s.NotaryGroup, s.PubSub)
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

	return &services.GenerateChainResponse{
		ChainId: id,
	}, nil
}

func (s *server) ExportChainTree(ctx context.Context, req *services.ExportChainRequest) (*services.ExportChainResponse, error) {
	session, err := NewSession(s.storagePath, req.Creds.WalletName, s.NotaryGroup, s.PubSub)
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

	return &services.ExportChainResponse{
		ChainTree: serializedChain,
	}, nil
}

func (s *server) ImportChainTree(ctx context.Context, req *services.ImportChainRequest) (*services.ImportChainResponse, error) {
	session, err := NewSession(s.storagePath, req.Creds.WalletName, s.NotaryGroup, s.PubSub)
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

	chain, err := session.ImportChain(req.ChainTree, !req.GetSkipValidation(), storageAdapterConfig)

	if err != nil {
		return nil, err
	}

	chainId, err := chain.Id()
	if err != nil {
		return nil, err
	}

	return &services.ImportChainResponse{
		ChainId: chainId,
	}, nil
}

func (s *server) ListChainIds(ctx context.Context, req *services.ListChainIdsRequest) (*services.ListChainIdsResponse, error) {
	session, err := NewSession(s.storagePath, req.Creds.WalletName, s.NotaryGroup, s.PubSub)
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

	return &services.ListChainIdsResponse{
		ChainIds: ids,
	}, nil
}

func (s *server) GetTip(ctx context.Context, req *services.GetTipRequest) (*services.GetTipResponse, error) {
	session, err := NewSession(s.storagePath, req.Creds.WalletName, s.NotaryGroup, s.PubSub)
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

	return &services.GetTipResponse{
		Tip: tipCid.String(),
	}, nil
}

func (s *server) GetTokenBalance(ctx context.Context, req *services.GetTokenBalanceRequest) (*services.GetTokenBalanceResponse, error) {
	session, err := NewSession(s.storagePath, req.Creds.WalletName, s.NotaryGroup, s.PubSub)
	if err != nil {
		return nil, err
	}

	err = session.Start(req.Creds.PassPhrase)
	if err != nil {
		return nil, fmt.Errorf("error starting session: %v", err)
	}

	defer session.Stop()

	amount, err := session.GetTokenBalance(req.ChainId, req.TokenName)
	if err != nil {
		return nil, err
	}

	return &services.GetTokenBalanceResponse{
		Amount: amount,
	}, nil
}

func (s *server) PlayTransactions(ctx context.Context, req *services.PlayTransactionsRequest) (*services.PlayTransactionsResponse, error) {
	session, err := NewSession(s.storagePath, req.Creds.WalletName, s.NotaryGroup, s.PubSub)
	if err != nil {
		return nil, err
	}

	err = session.Start(req.Creds.PassPhrase)
	if err != nil {
		return nil, fmt.Errorf("error starting session: %v", err)
	}

	defer session.Stop()

	resp, err := session.PlayTransactions(req.ChainId, req.KeyAddr, req.Transactions)
	if err != nil {
		return nil, fmt.Errorf("error playing transactions onto chaintree: %v", err)
	}

	return &services.PlayTransactionsResponse{
		Tip: resp.Tip.String(),
	}, nil
}

func (s *server) SetOwner(ctx context.Context, req *services.SetOwnerRequest) (*services.SetOwnerResponse, error) {
	session, err := NewSession(s.storagePath, req.Creds.WalletName, s.NotaryGroup, s.PubSub)
	if err != nil {
		return nil, err
	}

	err = session.Start(req.Creds.PassPhrase)
	if err != nil {
		return nil, fmt.Errorf("error starting session: %v", err)
	}

	defer session.Stop()

	txn := transactions.Transaction{
		Type:                transactions.Transaction_SETOWNERSHIP,
		SetOwnershipPayload: req.Payload,
	}

	blockResp, err := session.PlayTransactions(req.ChainId, req.KeyAddr, []*transactions.Transaction{&txn})
	if err != nil {
		return nil, fmt.Errorf("error transacting SetOwnership payload: %v", err)
	}

	return &services.SetOwnerResponse{
		Tip: blockResp.Tip.String(),
	}, nil
}

func (s *server) SetData(ctx context.Context, req *services.SetDataRequest) (*services.SetDataResponse, error) {
	session, err := NewSession(s.storagePath, req.Creds.WalletName, s.NotaryGroup, s.PubSub)
	if err != nil {
		return nil, err
	}

	err = session.Start(req.Creds.PassPhrase)
	if err != nil {
		return nil, fmt.Errorf("error starting session: %v", err)
	}

	defer session.Stop()

	txn := transactions.Transaction{
		Type:           transactions.Transaction_SETDATA,
		SetDataPayload: req.Payload,
	}

	blockResp, err := session.PlayTransactions(req.ChainId, req.KeyAddr, []*transactions.Transaction{&txn})
	if err != nil {
		return nil, fmt.Errorf("error transacting set data payload: %v", err)
	}

	return &services.SetDataResponse{
		Tip: blockResp.Tip.String(),
	}, nil
}

func (s *server) Resolve(ctx context.Context, req *services.ResolveRequest) (*services.ResolveResponse, error) {
	return s.resolveAt(ctx, req.Creds, req.ChainId, req.Path, nil)
}

func (s *server) ResolveAt(ctx context.Context, req *services.ResolveAtRequest) (*services.ResolveResponse, error) {
	t, err := cid.Decode(req.Tip)
	if err != nil {
		return nil, fmt.Errorf("error decoding tip: %v", err)
	}
	return s.resolveAt(ctx, req.Creds, req.ChainId, req.Path, &t)
}

func (s *server) resolveAt(ctx context.Context, creds *services.Credentials, chainId string, path string, tip *cid.Cid) (*services.ResolveResponse, error) {
	session, err := NewSession(s.storagePath, creds.WalletName, s.NotaryGroup, s.PubSub)
	if err != nil {
		return nil, err
	}

	err = session.Start(creds.PassPhrase)
	if err != nil {
		return nil, fmt.Errorf("error starting session: %v", err)
	}

	defer session.Stop()

	pathSegments, err := consensus.DecodePath(path)
	if err != nil {
		return nil, fmt.Errorf("bad path: %v", err)
	}

	data, remainingSegments, err := session.resolveAt(chainId, pathSegments, tip)
	if err != nil {
		return nil, err
	}

	remainingPath := strings.Join(remainingSegments, "/")
	dataBytes, err := cbornode.DumpObject(data)
	if err != nil {
		return nil, err
	}

	return &services.ResolveResponse{
		RemainingPath: remainingPath,
		Data:          dataBytes,
	}, nil
}

func (s *server) EstablishToken(ctx context.Context, req *services.EstablishTokenRequest) (*services.EstablishTokenResponse, error) {
	session, err := NewSession(s.storagePath, req.Creds.WalletName, s.NotaryGroup, s.PubSub)
	if err != nil {
		return nil, err
	}

	err = session.Start(req.Creds.PassPhrase)
	if err != nil {
		return nil, fmt.Errorf("error starting session: %v", err)
	}

	defer session.Stop()

	txn := transactions.Transaction{
		Type: transactions.Transaction_ESTABLISHTOKEN,
		EstablishTokenPayload: req.Payload,
	}

	blockResp, err := session.PlayTransactions(req.ChainId, req.KeyAddr, []*transactions.Transaction{&txn})
	if err != nil {
		return nil, fmt.Errorf("error transacting EstablishToken payload: %v", err)
	}

	return &services.EstablishTokenResponse{
		Tip: blockResp.Tip.String(),
	}, nil
}

func (s *server) MintToken(ctx context.Context, req *services.MintTokenRequest) (*services.MintTokenResponse, error) {
	session, err := NewSession(s.storagePath, req.Creds.WalletName, s.NotaryGroup, s.PubSub)
	if err != nil {
		return nil, err
	}

	err = session.Start(req.Creds.PassPhrase)
	if err != nil {
		return nil, fmt.Errorf("error starting session: %v", err)
	}

	defer session.Stop()

	txn := transactions.Transaction{
		Type:             transactions.Transaction_MINTTOKEN,
		MintTokenPayload: req.Payload,
	}

	blockResp, err := session.PlayTransactions(req.ChainId, req.KeyAddr, []*transactions.Transaction{&txn})
	if err != nil {
		return nil, fmt.Errorf("error transacting EstablishToken payload: %v", err)
	}

	return &services.MintTokenResponse{
		Tip: blockResp.Tip.String(),
	}, nil
}

func (s *server) SendToken(ctx context.Context, req *services.SendTokenRequest) (*services.SendTokenResponse, error) {
	session, err := NewSession(s.storagePath, req.Creds.WalletName, s.NotaryGroup, s.PubSub)
	if err != nil {
		return nil, err
	}

	err = session.Start(req.Creds.PassPhrase)
	if err != nil {
		return nil, fmt.Errorf("error starting session: %v", err)
	}

	defer session.Stop()

	sendToken, err := session.SendToken(req.ChainId, req.KeyAddr, req.TokenName, req.DestinationChainId, req.Amount)
	if err != nil {
		return nil, fmt.Errorf("error exporting token transfer payload: %v", err)
	}

	return &services.SendTokenResponse{
		SendToken: sendToken,
	}, nil
}

func (s *server) ReceiveToken(ctx context.Context, req *services.ReceiveTokenRequest) (*services.ReceiveTokenResponse, error) {
	session, err := NewSession(s.storagePath, req.Creds.WalletName, s.NotaryGroup, s.PubSub)
	if err != nil {
		return nil, err
	}

	err = session.Start(req.Creds.PassPhrase)
	if err != nil {
		return nil, fmt.Errorf("error starting session: %v", err)
	}

	defer session.Stop()

	tipCid, err := session.ReceiveToken(req.ChainId, req.KeyAddr, req.TokenPayload)
	if err != nil {
		return nil, err
	}

	return &services.ReceiveTokenResponse{
		Tip: tipCid.String(),
	}, nil
}

func startServer(grpcServer *grpc.Server, storagePath string, notaryGroup *types.NotaryGroup, pubsub remote.PubSub, port int) (*grpc.Server, error) {
	fmt.Println("Starting Tupelo RPC server")

	// By providing port 0 to net.Listen, we get a randomized one
	if port <= 0 {
		port = 0
	}
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		log.Printf("Failed to open listener: %s", err)
		return nil, fmt.Errorf("failed to open listener: %s", err)
	}

	if port == 0 {
		comps := strings.Split(listener.Addr().String(), ":")
		port, err = strconv.Atoi(comps[len(comps)-1])
		if err != nil {
			return nil, err
		}
	}

	s := &server{
		NotaryGroup: notaryGroup,
		PubSub:      pubsub,
		storagePath: storagePath,
	}

	services.RegisterWalletRPCServiceServer(grpcServer, s)
	reflection.Register(grpcServer)

	go func() {
		fmt.Println("Listening on port", port)
		err := grpcServer.Serve(listener)
		if err != nil {
			log.Printf("error serving: %v", err)
		}
	}()
	return grpcServer, nil
}

// Start gRPC unsecured server.
// Passing 0 for port means to pick any available port.
func ServeInsecure(storagePath string, notaryGroup *types.NotaryGroup, pubsub remote.PubSub, port int) (
	*grpc.Server, error) {
	grpcServer, err := startServer(grpc.NewServer(), storagePath, notaryGroup, pubsub, port)
	if err != nil {
		return nil, fmt.Errorf("error starting: %v", err)
	}
	return grpcServer, nil
}

// Start gRPC server secured with TLS.
// Passing 0 for port means to pick any available port.
func ServeTLS(storagePath string, notaryGroup *types.NotaryGroup, pubsub remote.PubSub, certFile string, keyFile string,
	port int) (*grpc.Server, error) {
	creds, err := credentials.NewServerTLSFromFile(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to create server TLS credentials from cert %s and key %s: %s",
			certFile, keyFile, err)
	}

	credsOption := grpc.Creds(creds)
	grpcServer := grpc.NewServer(credsOption)

	return startServer(grpcServer, storagePath, notaryGroup, pubsub, port)
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
