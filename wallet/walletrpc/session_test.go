package walletrpc

import (
	"encoding/base64"
	"os"
	"testing"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/gogo/protobuf/proto"
	"github.com/quorumcontrol/chaintree/safewrap"
	"github.com/quorumcontrol/messages/build/go/transactions"
	"github.com/quorumcontrol/storage"
	"github.com/quorumcontrol/tupelo/gossip3/actors"
	"github.com/quorumcontrol/tupelo/testnotarygroup"

	"github.com/Workiva/go-datastructures/bitarray"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/tupelo-go-sdk/consensus"
	"github.com/quorumcontrol/tupelo-go-sdk/conversion"
	extmsgs "github.com/quorumcontrol/tupelo-go-sdk/gossip3/messages"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/remote"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/types"
	"github.com/quorumcontrol/tupelo/wallet/adapters"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImportExport(t *testing.T) {
	path := ".tmp/test-import-export"
	err := os.RemoveAll(path)
	require.Nil(t, err)
	err = os.MkdirAll(path, 0755)
	require.Nil(t, err)
	defer os.RemoveAll(path)
	ng := types.NewNotaryGroup("importtest")
	ts := testnotarygroup.NewTestSet(t, 1)
	signer := types.NewLocalSigner(consensus.PublicKeyToEcdsaPub(&ts.PubKeys[0]), ts.SignKeys[0])
	pubSubSystem := remote.NewSimulatedPubSub()

	syncer, err := actor.EmptyRootContext.SpawnNamed(actors.NewTupeloNodeProps(&actors.TupeloConfig{
		Self:              signer,
		NotaryGroup:       ng,
		CurrentStateStore: storage.NewMemStorage(),
		PubSubSystem:      pubSubSystem,
	}), "tupelo-"+signer.ID)
	require.Nil(t, err)
	signer.Actor = syncer
	defer actor.EmptyRootContext.Poison(syncer)
	ng.AddSigner(signer)

	sess, err := NewSession(path, "test-only", ng, pubSubSystem)
	require.Nil(t, err)

	err = sess.CreateWallet("test")
	require.Nil(t, err)

	err = sess.Start("test")
	require.Nil(t, err)

	defer sess.Stop()

	key, err := sess.GenerateKey()
	require.Nil(t, err)

	addr := crypto.PubkeyToAddress(key.PublicKey).String()

	chain, err := sess.CreateChain(addr, &adapters.Config{Adapter: "mock"})
	require.Nil(t, err)

	sw := &safewrap.SafeWrap{}
	_, err = sess.SetData(chain.MustId(), addr, "test", sw.WrapObject("worked").RawData())
	require.Nil(t, err)

	export, err := sess.ExportChain(chain.MustId())
	require.Nil(t, err)

	imported, err := sess.ImportChain(export, true, &adapters.Config{Adapter: "mock"})
	require.Nil(t, err)

	assert.Equal(t, chain.MustId(), imported.MustId())

	// Test importing to different wallet
	sessNew, err := NewSession(path, "test-only-new", ng, pubSubSystem)
	require.Nil(t, err)

	err = sessNew.CreateWallet("test-new")
	require.Nil(t, err)

	err = sessNew.Start("test-new")
	require.Nil(t, err)

	defer sessNew.Stop()

	importedNew, err := sessNew.ImportChain(export, true, &adapters.Config{Adapter: "mock"})
	require.Nil(t, err)

	assert.Equal(t, chain.MustId(), importedNew.MustId())

	// Test importing to a wallet with a different notary group
	otherNg := types.NewNotaryGroup("importtest-other")
	sessOther, err := NewSession(path, "other-import-export", otherNg, pubSubSystem)
	require.Nil(t, err)

	err = sessOther.CreateWallet("test-other")
	require.Nil(t, err)

	err = sessOther.Start("test-other")
	require.Nil(t, err)

	defer sessOther.Stop()

	_, err = sessOther.ImportChain(export, true, &adapters.Config{Adapter: "mock"})
	require.NotNil(t, err)

	importedUnverified, err := sessOther.ImportChain(export, false, &adapters.Config{Adapter: "mock"})
	require.Nil(t, err)
	assert.Equal(t, chain.MustId(), importedUnverified.MustId())
}

func TestSendToken(t *testing.T) {
	path := ".tmp/test-send-token"
	err := os.RemoveAll(path)
	require.Nil(t, err)
	err = os.MkdirAll(path, 0755)
	require.Nil(t, err)
	defer os.RemoveAll(path)
	ng := types.NewNotaryGroup("send-token-test")
	ts := testnotarygroup.NewTestSet(t, 1)
	signer := types.NewLocalSigner(consensus.PublicKeyToEcdsaPub(&ts.PubKeys[0]), ts.SignKeys[0])
	pubSubSystem := remote.NewSimulatedPubSub()

	syncer, err := actor.EmptyRootContext.SpawnNamed(actors.NewTupeloNodeProps(&actors.TupeloConfig{
		Self:              signer,
		NotaryGroup:       ng,
		CurrentStateStore: storage.NewMemStorage(),
		PubSubSystem:      pubSubSystem,
	}), "tupelo-"+signer.ID)
	require.Nil(t, err)
	signer.Actor = syncer
	defer actor.EmptyRootContext.Poison(syncer)
	ng.AddSigner(signer)

	sess, err := NewSession(path, "send-token-test", ng, pubSubSystem)
	require.Nil(t, err)

	err = sess.CreateWallet("test")
	require.Nil(t, err)

	err = sess.Start("test")
	require.Nil(t, err)

	defer sess.Stop()

	key, err := sess.GenerateKey()
	require.Nil(t, err)

	addr := crypto.PubkeyToAddress(key.PublicKey).String()

	chain, err := sess.CreateChain(addr, &adapters.Config{Adapter: "mock"})
	require.Nil(t, err)

	destKey, err := sess.GenerateKey()
	require.Nil(t, err)

	destAddr := crypto.PubkeyToAddress(destKey.PublicKey).String()

	destChain, err := sess.CreateChain(destAddr, &adapters.Config{Adapter: "mock"})
	require.Nil(t, err)

	_, err = sess.EstablishToken(chain.MustId(), addr, "test-token", 1000000)
	require.Nil(t, err)

	_, err = sess.MintToken(chain.MustId(), addr, "test-token", 10)
	require.Nil(t, err)

	sendTokens, err := sess.SendToken(chain.MustId(), addr, "test-token", destChain.MustId(), 5)
	require.Nil(t, err)
	decodedSendTokens, err := base64.StdEncoding.DecodeString(sendTokens)
	require.Nil(t, err)
	unmarshalledSendTokens := &transactions.TokenPayload{}
	err = proto.Unmarshal(decodedSendTokens, unmarshalledSendTokens)
	require.Nil(t, err)

	// TODO: Improve these assertions
	assert.NotEmpty(t, unmarshalledSendTokens.TransactionId)
	assert.NotEmpty(t, unmarshalledSendTokens.Leaves)
	assert.NotNil(t, unmarshalledSendTokens.Tip)
	assert.NotNil(t, unmarshalledSendTokens.Signature)
}

func TestGetTip(t *testing.T) {
	path := ".tmp/get-tip-test"
	err := os.RemoveAll(path)
	require.Nil(t, err)
	err = os.MkdirAll(path, 0755)
	require.Nil(t, err)
	defer os.RemoveAll(path)

	ng := types.NewNotaryGroup("get-tip-test")
	ts := testnotarygroup.NewTestSet(t, 1)
	signer := types.NewLocalSigner(consensus.PublicKeyToEcdsaPub(&ts.PubKeys[0]), ts.SignKeys[0])
	pubSubSystem := remote.NewSimulatedPubSub()

	syncer, err := actor.EmptyRootContext.SpawnNamed(actors.NewTupeloNodeProps(&actors.TupeloConfig{
		Self:              signer,
		NotaryGroup:       ng,
		CurrentStateStore: storage.NewMemStorage(),
		PubSubSystem:      pubSubSystem,
	}), "tupelo-"+signer.ID)
	require.Nil(t, err)
	signer.Actor = syncer
	defer actor.EmptyRootContext.Poison(syncer)
	ng.AddSigner(signer)

	sess, err := NewSession(path, "get-tip-test", ng, pubSubSystem)
	require.Nil(t, err)

	err = sess.CreateWallet("test")
	require.Nil(t, err)

	err = sess.Start("test")
	require.Nil(t, err)

	defer sess.Stop()

	key, err := sess.GenerateKey()
	require.Nil(t, err)

	addr := crypto.PubkeyToAddress(key.PublicKey).String()

	chain, err := sess.CreateChain(addr, &adapters.Config{Adapter: "mock"})
	require.Nil(t, err)

	sw := &safewrap.SafeWrap{}

	newTip, err := sess.SetData(chain.MustId(), addr, "test", sw.WrapObject("worked").RawData())
	require.Nil(t, err)

	getTipResp, err := sess.GetTip(chain.MustId())
	require.Nil(t, err)

	require.Equal(t, newTip, getTipResp)
}

func TestSerializeDeserializeSignature(t *testing.T) {
	signers := bitarray.NewBitArray(3)
	err := signers.SetBit(0)
	require.Nil(t, err)
	err = signers.SetBit(2)
	require.Nil(t, err)

	marshalledSigners, err := bitarray.Marshal(signers)
	require.Nil(t, err)

	intSig := extmsgs.Signature{
		TransactionID: nil,
		ObjectID:      []byte("objectid"),
		PreviousTip:   []byte("previousTip"),
		NewTip:        []byte("newtip"),
		View:          1,
		Cycle:         2,
		Height:        3,
		Type:          "test",
		Signers:       marshalledSigners,
		Signature:     []byte("signature"),
	}

	encoded, err := conversion.ToInternalSignature(intSig)
	require.Nil(t, err)
	decoded, err := conversion.ToExternalSignature(encoded)
	require.Nil(t, err)

	require.Equal(t, intSig, *decoded)
}

func testGetTokenBalance(t *testing.T, sess *RPCSession, chain *consensus.SignedChainTree) {
	bal, err := sess.GetTokenBalance(chain.MustId(), "test-token")
	require.Nil(t, err)
	require.Equal(t, uint64(10), bal)
}

func testGetNonExistentTokenBalance(t *testing.T, sess *RPCSession,
	chain *consensus.SignedChainTree) {
	bal, err := sess.GetTokenBalance(chain.MustId(), "non-existent-token")
	require.NotNil(t, err)
	require.Equal(t, uint64(0), bal)
}

func TestTokens(t *testing.T) {
	testCases := []struct {
		name   string
		testFn func(*testing.T, *RPCSession, *consensus.SignedChainTree)
	}{
		{"get existing token balance", testGetTokenBalance},
		{"get non-existent token balance", testGetNonExistentTokenBalance},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			path := ".tmp/test-tokens"
			err := os.RemoveAll(path)
			require.Nil(t, err)
			err = os.MkdirAll(path, 0755)
			require.Nil(t, err)
			defer os.RemoveAll(path)

			ng := types.NewNotaryGroup("test-tokens")
			ts := testnotarygroup.NewTestSet(t, 1)
			signer := types.NewLocalSigner(consensus.PublicKeyToEcdsaPub(&ts.PubKeys[0]), ts.SignKeys[0])
			pubSubSystem := remote.NewSimulatedPubSub()

			syncer, err := actor.EmptyRootContext.SpawnNamed(actors.NewTupeloNodeProps(&actors.TupeloConfig{
				Self:              signer,
				NotaryGroup:       ng,
				CurrentStateStore: storage.NewMemStorage(),
				PubSubSystem:      pubSubSystem,
			}), "tupelo-"+signer.ID)
			require.Nil(t, err)
			signer.Actor = syncer
			defer actor.EmptyRootContext.Poison(syncer)
			ng.AddSigner(signer)

			sess, err := NewSession(path, "test-tokens", ng, pubSubSystem)
			require.Nil(t, err)

			err = sess.CreateWallet("test")
			require.Nil(t, err)

			err = sess.Start("test")
			require.Nil(t, err)

			defer sess.Stop()

			key, err := sess.GenerateKey()
			require.Nil(t, err)
			addr := crypto.PubkeyToAddress(key.PublicKey).String()
			chain, err := sess.CreateChain(addr, &adapters.Config{Adapter: "mock"})
			require.Nil(t, err)

			_, err = sess.EstablishToken(chain.MustId(), addr, "test-token", 1000000)
			require.Nil(t, err)

			_, err = sess.MintToken(chain.MustId(), addr, "test-token", 10)
			require.Nil(t, err)

			tc.testFn(t, sess, chain)
		})
	}
}
