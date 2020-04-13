# Tupelo

[![Contributor Covenant](https://img.shields.io/badge/Contributor%20Covenant-v2.0%20adopted-ff69b4.svg)](CODE_OF_CONDUCT.md)

## Docs
Tupelo docs are available at [https://docs.tupelo.org/](https://docs.tupelo.org/)

## Repo Structure
For using the Tupelo network, [see the `sdk/` directory](sdk/).

For running a Tupelo signer, [see the `signer/` directory](signer/).

## Concepts
Every asset and actor in the system has their own Chain Tree, a new data structure combining a merkle-DAG and an individual ordered log of transactions. A Chain Tree is similar in concept to Git, but with known transactions on data instead of simple textual manipulation. Functionally, a Chain Tree is a state-machine where the input and resulting state is a content-addressable merkle-DAG. Playing ordered transactions on an existing state produces a new state.

A group of Signers (as part of a Notary Group) keep track of the current state of every Chain Tree in order to prevent forking. The Notary Group does not need to keep the entire history nor the entire state of every Chain Tree, but only the latest tip (a hash of the current root node). Therefore, each owner of a Chain Tree is responsible for the storage of that Chain Tree. This provides flexibility for the owner to utilize existing storage backends, public distributed storage, or even local cold storage.

The Chain Tree DAG is modeled using the IPLD format, a specification describing linked, content-addressable, json-like data structures. Using this standardized structure allows quick and easy traversal of the data within the Chain Tree while maintaining schema flexibility. An individual IPLD node is similar to single key / value json document, however these values can be further links to other IPLD nodes, creating a DAG structure. In Tupelo, all nested objects are set as links to other IPLD nodes, which allows the user to fetch only the subset of the Chain Tree they need.

#### Example
Let’s take the scenario of Alice buying a car from VW to demonstrate how the protocol works:

Every Chain Tree's genesis state begins with a public/private key pair and a deterministic name of `did:tupelo:<hex-encoded-genesis-public-key>`. So for this example, VW creates a new Chain Tree with their public/private key and publishes the genesis block:
```
ChainId: did:tupelo:<hex-encoded-genesis-public-key>
Transactions: [
  {
    Type: SET_DATA,
    Payload: {
      path: "vin",
      value: <hash_of_VIN_number>
    }
  }
]
Signatures: [<VWs secp256k1 generated key>]
```

Upon publishing this block, VW awaits a response indicating the block has been signed by the Notary Group. It is important to highlight that the data involved in this transaction is opaque to the protocol, so VW can structure this payload in any way that is reasonable for its use case. In addition, VW can use the transaction internally to associate this Chain Tree with an individual vehicle in their systems.

After the genesis block, VW can continue to perform transactions on that Chain Tree as they see fit:
```
ChainId: did:tupelo:<hex-encoded-genesis-public-key>
PreviousTip: <tip returned by the genesis block>
Transactions: [
  {
    Type: SET_DATA,
    Payload: {
      path: "production/completed_date",
      value: "2015-10-21"
    }
  },
  {
    Type: SET_DATA,
    Payload: {
      path: "production/batch",
      value: "123857123409"
    }
  }
]
Signatures: [<VWs secp256k1 generated key>]
```

At this point, the Chain Tree's resolved merkle-DAG would look like this:
```
{
  vin: <hash_of_VIN_number>
  production: {
    completed_date: "2015-10-21"
    batch: "123857123409"
  }
}
```

Alice buys the car and requests ownership from VW. Off-chain Alice gives VW a public key (or many) and VW publishes a new block to the p2p network:
```
ChainId: did:tupelo:<hex-encoded-genesis-public-key>
PreviousTip: <tip returned by the last transaction>
Transactions: [
  {
    Type: SET_OWNERSHIP,
    Payload: {
      authentication: [<alice public key>]
    }
  }
]
Signatures: [<VWs secp256k1 generated key>]
```

After the transaction is confirmed, Alice now owns the Chain Tree for that vehicle. She does not even need to be online to verify that ownership as she will have an off-line list of the Notary Group public keys. Alice now has complete control of this Chain Tree, consequently VW no longer has the ability to submit transactions for that Chain Tree.
