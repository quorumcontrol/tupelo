# Tupelo Technical Manual
This is the technical manual for the Tupelo distributed ledger technology (DLT for short).

## About Tupelo
Tupelo is a peer to peer system, based around the chain tree concept. Each "chain tree" is
a directed acyclic graph (DAG) of state, which get produced through a sequence of transactions,
each of which gets agreed upon by a group of signer nodes. Since individual transactions
get accepted through network concensus, the current state of a tree (i.e. the "tip") is
also commonly agreed upon.

Peers that wish to connect to the network bootstrap via so-called bootstrap nodes, which
let them e.g. submit transactions for signing by signer nodes.

## Client Usage
A Tupelo client that wishes to work with a chain tree, will first bootstrap with the network
by calling `p2p.LibP2PHost.Bootstrap` supplying the addresses of the bootstrap nodes.

Then it will set up a notary group, using the public keys of the signer nodes. By calling
`gossip3/types.NotaryGroup.SetupAllRemoteActors`, each of the signer objects will also have
a remote actor representation in order to interact with the corresponding remote signer.

### Adding a Chain Tree Transaction
In order for a client to update a chain tree by sending a transaction, it must first of all have a
copy of it. A Go implementation might for example have a
[Badger](https://github.com/dgraph-io/badger) database based storage adapter, where it keeps
a serialized representation of the chain tree between sessions.

Before constructing a transaction, the client will deserialize a chain tree representation from
the representation in storage. Then the transaction, e.g. to set a data property, will be
constructed given the tip of the chain tree. The transaction will afterwards be broadcasted for
signing off by signer nodes.
