package walletrpc

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"github.com/improbable-eng/grpc-web/go/grpcweb"
	cid "github.com/ipfs/go-cid"
	cbornode "github.com/ipfs/go-ipld-cbor"
	"github.com/quorumcontrol/chaintree/chaintree"
	gossip3client "github.com/quorumcontrol/tupelo-go-client/client"
	"github.com/quorumcontrol/tupelo-go-client/consensus"
	"github.com/quorumcontrol/tupelo/wallet"
	"github.com/quorumcontrol/tupelo/wallet/adapters"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
)

const (
	defaultWebPort    = ":50050"
	walletMetaKey     = "wallet"
	passphraseMetaKey = "passphrase"
)

type server struct {
	client      *gossip3client.Client
	storagePath string
}

type walletCredentials struct {
	wallet     string
	passphrase string
}

func getWalletCredentials(ctx context.Context) (*walletCredentials, error) {
	meta, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, fmt.Errorf("error getting wallet credentials")
	}

	w, ok := meta[walletMetaKey]
	if !ok {
		return nil, fmt.Errorf("no wallet name in credentials")
	}

	p, ok := meta[passphraseMetaKey]
	if !ok {
		return nil, fmt.Errorf("no passphrase in credentials")
	}

	return &walletCredentials{
		wallet:     w[0],
		passphrase: p[0],
	}, nil
}

func (s *server) Register(ctx context.Context, req *RegisterWalletRequest) (*RegisterWalletResponse, error) {
	creds, err := getWalletCredentials(ctx)
	if err != nil {
		return nil, err
	}

	session, err := NewSession(s.storagePath, creds.wallet, s.client)
	if err != nil {
		return nil, err
	}
	defer session.Stop()

	err = session.CreateWallet(creds.passphrase)
	if err != nil {
		if _, ok := err.(*wallet.CreateExistingWalletError); ok {
			msg := fmt.Sprintf("wallet %v already exists", creds.wallet)
			return nil, status.Error(codes.AlreadyExists, msg)
		}
		return nil, fmt.Errorf("error creating wallet: %v", err)
	}

	return &RegisterWalletResponse{
		WalletName: creds.wallet,
	}, nil
}

func (s *server) GenerateKey(ctx context.Context, req *GenerateKeyRequest) (*GenerateKeyResponse, error) {
	creds, err := getWalletCredentials(ctx)
	if err != nil {
		return nil, err
	}

	session, err := NewSession(s.storagePath, creds.wallet, s.client)
	if err != nil {
		return nil, err
	}

	err = session.Start(creds.passphrase)
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
	creds, err := getWalletCredentials(ctx)
	if err != nil {
		return nil, err
	}

	session, err := NewSession(s.storagePath, creds.wallet, s.client)
	if err != nil {
		return nil, err
	}

	err = session.Start(creds.passphrase)
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
	creds, err := getWalletCredentials(ctx)
	if err != nil {
		return nil, err
	}

	session, err := NewSession(s.storagePath, creds.wallet, s.client)
	if err != nil {
		return nil, err
	}

	err = session.Start(creds.passphrase)
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
	creds, err := getWalletCredentials(ctx)
	if err != nil {
		return nil, err
	}

	session, err := NewSession(s.storagePath, creds.wallet, s.client)
	if err != nil {
		return nil, err
	}

	err = session.Start(creds.passphrase)
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
	creds, err := getWalletCredentials(ctx)
	if err != nil {
		return nil, err
	}

	session, err := NewSession(s.storagePath, creds.wallet, s.client)
	if err != nil {
		return nil, err
	}

	err = session.Start(creds.passphrase)
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
	creds, err := getWalletCredentials(ctx)
	if err != nil {
		return nil, err
	}

	session, err := NewSession(s.storagePath, creds.wallet, s.client)
	if err != nil {
		return nil, err
	}

	err = session.Start(creds.passphrase)
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
	creds, err := getWalletCredentials(ctx)
	if err != nil {
		return nil, err
	}

	session, err := NewSession(s.storagePath, creds.wallet, s.client)
	if err != nil {
		return nil, err
	}

	err = session.Start(creds.passphrase)
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

func buildTransactions(protoTransactions []*ProtoTransaction) ([]*chaintree.Transaction, error) {
	chaintreeTxns := make([]*chaintree.Transaction, len(protoTransactions))
	for i, protoTxn := range protoTransactions {
		switch protoTxn.Type {
		case ProtoTransaction_ESTABLISHTOKEN:
			payload := protoTxn.GetEstablishTokenPayload()
			chaintreeTxns[i] = &chaintree.Transaction{
				Type: consensus.TransactionTypeEstablishToken,
				Payload: consensus.EstablishTokenPayload{
					Name: payload.Name,
					MonetaryPolicy: consensus.TokenMonetaryPolicy{
						Maximum: payload.MonetaryPolicy.Maximum,
					},
				},
			}
		case ProtoTransaction_MINTTOKEN:
			payload := protoTxn.GetMintTokenPayload()
			chaintreeTxns[i] = &chaintree.Transaction{
				Type: consensus.TransactionTypeMintToken,
				Payload: consensus.MintTokenPayload{
					Name:   payload.Name,
					Amount: payload.Amount,
				},
			}
		case ProtoTransaction_SETDATA:
			payload := protoTxn.GetSetDataPayload()

			var decodedVal interface{}
			err := cbornode.DecodeInto(payload.Value, &decodedVal)
			if err != nil {
				return nil, fmt.Errorf("error decoding value: %v", err)
			}

			chaintreeTxns[i] = &chaintree.Transaction{
				Type: consensus.TransactionTypeSetData,
				Payload: consensus.SetDataPayload{
					Path:  payload.Path,
					Value: decodedVal,
				},
			}
		case ProtoTransaction_SETOWNERSHIP:
			payload := protoTxn.GetSetOwnershipPayload()
			chaintreeTxns[i] = &chaintree.Transaction{
				Type: consensus.TransactionTypeSetOwnership,
				Payload: consensus.SetOwnershipPayload{
					Authentication: payload.Authentication,
				},
			}
		default:
			return nil, fmt.Errorf("unrecognized transaction type: %v", protoTxn.Type)
		}
	}

	return chaintreeTxns, nil
}

func (s *server) PlayTransactions(ctx context.Context, req *PlayTransactionsRequest) (*PlayTransactionsResponse, error) {
	creds, err := getWalletCredentials(ctx)
	if err != nil {
		return nil, err
	}

	session, err := NewSession(s.storagePath, creds.wallet, s.Client)
	if err != nil {
		return nil, err
	}

	err = session.Start(creds.passphrase)
	if err != nil {
		return nil, fmt.Errorf("error starting session: %v", err)
	}

	defer session.Stop()

	transactions, err := buildTransactions(req.Transactions)
	if err != nil {
		return nil, err
	}

	resp, err := session.PlayTransactions(req.ChainId, req.KeyAddr, transactions)
	if err != nil {
		return nil, err
	}

	return &PlayTransactionsResponse{
		Tip: resp.Tip.String(),
	}, nil
}

func (s *server) SetOwner(ctx context.Context, req *SetOwnerRequest) (*SetOwnerResponse, error) {
	creds, err := getWalletCredentials(ctx)
	if err != nil {
		return nil, err
	}

	session, err := NewSession(s.storagePath, creds.wallet, s.client)
	if err != nil {
		return nil, err
	}

	err = session.Start(creds.passphrase)
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
	creds, err := getWalletCredentials(ctx)
	if err != nil {
		return nil, err
	}

	session, err := NewSession(s.storagePath, creds.wallet, s.client)
	if err != nil {
		return nil, err
	}

	err = session.Start(creds.passphrase)
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
	creds, err := getWalletCredentials(ctx)
	if err != nil {
		return nil, err
	}

	return s.resolveAt(ctx, creds, req.ChainId, req.Path, nil)
}

func (s *server) ResolveAt(ctx context.Context, req *ResolveAtRequest) (*ResolveResponse,
	error) {
	creds, err := getWalletCredentials(ctx)
	if err != nil {
		return nil, err
	}

	t, err := cid.Decode(req.Tip)
	if err != nil {
		return nil, fmt.Errorf("A valid tip CID must be provided")
	}
	return s.resolveAt(ctx, creds, req.ChainId, req.Path, &t)
}

func (s *server) resolveAt(ctx context.Context, creds *walletCredentials, chainId string, path string,
	tip *cid.Cid) (*ResolveResponse, error) {
	session, err := NewSession(s.storagePath, creds.WalletName, s.Client)

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

	return &ResolveResponse{
		RemainingPath: remainingPath,
		Data:          dataBytes,
	}, nil
}

func (s *server) EstablishToken(ctx context.Context, req *EstablishTokenRequest) (*EstablishTokenResponse, error) {
	creds, err := getWalletCredentials(ctx)
	if err != nil {
		return nil, err
	}

	session, err := NewSession(s.storagePath, creds.wallet, s.client)
	if err != nil {
		return nil, err
	}

	err = session.Start(creds.passphrase)
	if err != nil {
		return nil, fmt.Errorf("error starting session: %v", err)
	}

	defer session.Stop()

	tipCid, err := session.EstablishToken(req.ChainId, req.KeyAddr, req.TokenName, req.Maximum)
	if err != nil {
		return nil, err
	}

	return &EstablishTokenResponse{
		Tip: tipCid.String(),
	}, nil
}

func (s *server) MintToken(ctx context.Context, req *MintTokenRequest) (*MintTokenResponse, error) {
	creds, err := getWalletCredentials(ctx)
	if err != nil {
		return nil, err
	}

	session, err := NewSession(s.storagePath, creds.wallet, s.client)
	if err != nil {
		return nil, err
	}

	err = session.Start(creds.passphrase)
	if err != nil {
		return nil, fmt.Errorf("error starting session: %v", err)
	}

	defer session.Stop()

	tipCid, err := session.MintToken(req.ChainId, req.KeyAddr, req.TokenName, req.Amount)
	if err != nil {
		return nil, err
	}

	return &MintTokenResponse{
		Tip: tipCid.String(),
	}, nil
}

func (s *server) SendToken(ctx context.Context, req *SendTokenRequest) (*SendTokenResponse, error) {
	creds, err := getWalletCredentials(ctx)
	if err != nil {
		return nil, err
	}

	session, err := NewSession(s.storagePath, creds.wallet, s.Client)
	if err != nil {
		return nil, err
	}

	err = session.Start(creds.passphrase)
	if err != nil {
		return nil, fmt.Errorf("error starting session: %v", err)
	}

	defer session.Stop()

	sendToken, err := session.SendToken(req.ChainId, req.KeyAddr, req.TokenName, req.DestinationChainId, req.Amount)
	if err != nil {
		return nil, err
	}

	return &SendTokenResponse{
		SendToken: sendToken,
	}, nil
}

func (s *server) ReceiveToken(ctx context.Context, req *ReceiveTokenRequest) (*ReceiveTokenResponse, error) {
	creds, err := getWalletCredentials(ctx)
	if err != nil {
		return nil, err
	}

	session, err := NewSession(s.storagePath, creds.wallet, s.Client)
	if err != nil {
		return nil, err
	}

	err = session.Start(creds.passphrase)
	if err != nil {
		return nil, fmt.Errorf("error starting session: %v", err)
	}

	defer session.Stop()

	tipCid, err := session.ReceiveToken(req.ChainId, req.KeyAddr, req.TokenPayload)
	if err != nil {
		return nil, err
	}

	return &ReceiveTokenResponse{
		Tip: tipCid.String(),
	}, nil
}

func startServer(grpcServer *grpc.Server, storagePath string, client *gossip3client.Client, port int) (*grpc.Server, error) {
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
		client:      client,
		storagePath: storagePath,
	}

	RegisterWalletRPCServiceServer(grpcServer, s)
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
func ServeInsecure(storagePath string, client *gossip3client.Client, port int) (
	*grpc.Server, error) {
	grpcServer, err := startServer(grpc.NewServer(), storagePath, client, port)
	if err != nil {
		return nil, fmt.Errorf("error starting: %v", err)
	}
	return grpcServer, nil
}

// Start gRPC server secured with TLS.
// Passing 0 for port means to pick any available port.
func ServeTLS(storagePath string, client *gossip3client.Client, certFile string, keyFile string,
	port int) (*grpc.Server, error) {
	creds, err := credentials.NewServerTLSFromFile(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to create server TLS credentials from cert %s and key %s: %s",
			certFile, keyFile, err)
	}

	credsOption := grpc.Creds(creds)
	grpcServer := grpc.NewServer(credsOption)

	return startServer(grpcServer, storagePath, client, port)
}

func ServeWebInsecure(ctx context.Context, grpcServer *grpc.Server) (*http.Server, error) {
	opts := []grpc.DialOption{grpc.WithInsecure()}
	s, err := createGrpcWeb(ctx, grpcServer, defaultWebPort, opts)
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

func ServeWebTLS(ctx context.Context, grpcServer *grpc.Server, certFile string, keyFile string) (*http.Server, error) {
	creds, err := credentials.NewClientTLSFromFile(certFile, grpcHost())
	if err != nil {
		return nil, fmt.Errorf("error loading TLS credentials: %v", err)
	}

	opts := []grpc.DialOption{grpc.WithTransportCredentials(creds)}

	s, err := createGrpcWeb(ctx, grpcServer, defaultWebPort, opts)
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

func createGrpcWeb(ctx context.Context, grpcServer *grpc.Server, port string, opts []grpc.DialOption) (*http.Server, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	httpHandler, err := httpProxyHandler(ctx, port, opts)
	if err != nil {
		return nil, err
	}

	webWrappedGrpc := grpcweb.WrapServer(grpcServer)

	handler := http.HandlerFunc(func(resp http.ResponseWriter, req *http.Request) {
		if webWrappedGrpc.IsGrpcWebRequest(req) {
			webWrappedGrpc.ServeHTTP(resp, req)
			return
		} else if req.Method == "OPTIONS" {
			headers := resp.Header()
			headers.Add("Access-Control-Allow-Origin", "*")
			headers.Add("Access-Control-Allow-Headers", "*")
			headers.Add("Access-Control-Allow-Methods", "GET, PUT, POST, OPTIONS")
			resp.WriteHeader(http.StatusOK)
		} else {
			httpHandler.ServeHTTP(resp, req)
			return
		}
	})

	s := &http.Server{
		Addr:    port,
		Handler: handler,
	}
	return s, nil
}

func httpProxyHandler(ctx context.Context, port string, opts []grpc.DialOption) (*runtime.ServeMux, error) {
	rpcHost := grpcHost(port)

	credMetadata := runtime.WithMetadata(func(ctx context.Context, req *http.Request) metadata.MD {
		wallet, passphrase, _ := req.BasicAuth()
		return metadata.Pairs(walletMetaKey, wallet, passphraseMetaKey, passphrase)
	})
	mux := runtime.NewServeMux(credMetadata)

	err := RegisterWalletRPCServiceHandlerFromEndpoint(ctx, mux, rpcHost, opts)
	if err != nil {
		return nil, fmt.Errorf("error starting HTTP proxy server: %v", err)
	}

	return mux, nil
}

func grpcHost(port string) string {
	return "localhost:" + port
}
