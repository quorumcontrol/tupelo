# Tupelo Technical Manual
This is the technical manual for the Tupelo distributed ledger technology (DLT for short).

## About Tupelo
Tupelo is a peer to peer system, based around the "chain tree" concept. Each chain tree is
a directed acyclic graph (DAG) of state, which gets produced through a sequence of transaction
blocks, each of which gets agreed upon by a group of signer nodes. Since individual transactions
get accepted through network concensus, the current state of a tree (i.e. the "tip") is
also commonly agreed upon.

Peers that wish to join the network connect via so-called bootstrap nodes. After bootstrapping,
they can submit transactions for signing by signer nodes, by publishing messages through libp2p
pubsub.

## Client Usage
A Tupelo client that wishes to work with a chain tree, will first bootstrap with the network
by calling `p2p.LibP2PHost.Bootstrap` supplying the addresses of the bootstrap nodes.

Then it will set up a notary group, using the public keys of the signer nodes. By calling
`gossip3/types.NotaryGroup.SetupAllRemoteActors`, each of the signer objects will also have
a remote actor representation in order to interact with the corresponding remote signer.

### Adding Chain Tree Transactions
In order for a client to update a chain tree by sending one or more transactions, it must first
of all have a copy of it. A Go implementation might for example have a 
[Badger](https://github.com/dgraph-io/badger) database based storage adapter, where it keeps
a serialized representation of the chain tree between sessions.

Before constructing transactions, the client will deserialize a chain tree representation from
the representation in storage. Then one or more transactions, e.g. to set a data property, will be
constructed given the tip of the chain tree. The transaction block will afterwards be broadcasted
for signing off by signer nodes.

## Signer Behaviour
Signer nodes listen for messages on a libp2p pubsub system, that are for a certain topic.
Once nodes receive such messages, they are passed on to the Tupelo actor which processes
them according to their type. The different types of messages are discussed beneath:

### AddBlockRequest
`AddBlockRequest` messages are sent by clients in order to execute a set of transactions, 
which Tupelo encapsulates as a so-called block. Once a signer receives a message of this type,
it will schedule a validation of the block. A validator actor proceeds to validate the block
and if it can find the corresponding chain tree in its storage, it will check if the
block is for the expected transaction height; otherwise, if it's higher than expected,
it will be marked as preflight, if it's lower than expected, it will be marked as stale. If
the corresponding chain tree isn't found in storage, the block is marked as preflight.

If a block passes all the aforementioned checks and is thus validated, it gets marked as accepted
and the Tupelo actor receives a `TransactionWrapper` message wrapping the `AddBlockRequest`.

### TransactionWrapper
The Tupelo actor receives `TransactionWrapper` messages from validator actors, in response
to requests to validate `AddBlockRequest` messages. The Tupelo actor sends them on to
the conflict set router actor. Said actor first checks if a conflict set should be created for the 
message or not, identified by corresponding chain tree and height. If not (i.e., it's already been 
processed), the actor simply ignores the message, otherwise it creates a conflict set
representation and requests that one of its workers process it.

A worker processes a conflict set by obtaining a signature, then checking if the conflict set
has enough signatures (>= quorum count). If it does, a `CurrentStateWrapper` message representing
the updated current state is created and sent to the conflict set router. The latter
proceeds to broadcast the message to the network (so other signers can sync), and also
prunes conflict sets for the same chain tree and with heights lower than or equal to the 
current one.

### ValidateTransaction
The Tupelo actor receives `ValidateTransaction` messages from conflict set workers, when
snoozed transactions need to be validated as part of conflict set activation. This is in turn
triggered by the Tupelo actor sending `ActivateSnoozingConflictSets` to the conflict set
router when a `CurrentStateWrapper` message is received, i.e. a new current state has been
committed.

The Tupelo actor responds to this message by sending a corresponding `validationRequest` message
to the validator pool.

### CurrentStateWrapper
When the Tupelo actor receives a `CurrentStateWrapper` message, it first of all stores the
updated current state for the chain tree ID. Then it sends an `ActivateSnoozingConflictSets`
message to the conflict set router, in order to activate snoozed conflict sets.

The reason for activating snoozed conflict sets here is that when a new chain tree state
is reached, we will want to re-process conflict sets with snoozed (previously) pre-flight
transactions or with a snoozed current state commit.

If a conflict set in this situation has a snoozed current state commit, it gets processed
and if the commit is of the correct height the conflict set is activated and marked as done.

Otherwise, if the conflict set is _not_ considered done, any transactions it has are 
re-validated, by sending corresponding `ValidateTransaction` messages to the conflict set router,
which forwards it to the Tupelo actor.

If the `CurrentStateWrapper` message emanated from within the node, the updated state is
broadcasted via pubsub in order to notify other signers. 

### Signature
`Signature` messages get forwarded by the Tupelo actor to the conflict set router, which
determines the corresponding conflict set and forwards the message to a corresponding worker
if the conflict set isn't already done.

If the signature emanates from within the node, the conflict set worker will forward it to other
signers. Then, the worker will record the new signature for the transaction in question
and increment the conflict set's number of updates. Finally, the worker will send a
`checkStateMsg` to itself, to trigger a state check.

Upon receipt of a `checkStateMsg`, a conflict set worker will first of all check if the message's
update counter is at least as high as that of the conflict set. If it isn't, it's considered
stale and simply ignored. Then the worker goes on to check if the conflict set is done;
if it has a transaction block with enough signatures (>= quorum count), it's considered done and
a new current state is committed from the transaction block in question. A corresponding 
`CurrentStateWrapper` message is sent to the conflict set router in order to propagate the
state change.

### GetTipRequest
TODO

### RequestCurrentStateSnapshot
TODO

## Monitoring
Tupelo supports monitoring with Prometheus. In order to accomplish this it uses the standard
Prometheus [client library](https://github.com/prometheus/client_golang) to expose
metrics via an HTTP server, and also to produce custom metrics.

### Metrics
Tupelo produces the following Prometheus metrics:

#### Total Number of Transactions
A counter of total number of transactions handled.

#### Current Number of Chain Trees
A gauge of current number of chain trees. A chain tree is created by issuing a genesis
transaction. When a signer node receives such a transaction, it increases the aforementioned
gauge.

#### Number of Currently Snoozed Conflict Sets
A gauge of current number of snoozed conflict sets.
