package api

var Schema = `
scalar JSON

type Block {
	data: String! # base64
}

type AddBlockPayload {
	valid: Boolean!
	newTip: String! # base64 todo: CID scalar
	newBlocks: [Block!]
}

type ResolvePayload {
	remainingPath: [String!]!
	value: JSON
}

input ResolveInput {
	did: String!
	path: String!
}

input AddBlockInput {
  addBlockRequest: String! # The serialized protobuf as base64
}

type Query {
  resolve(input:ResolveInput!):ResolvePayload
}

type Mutation {
  # This mutation takes id and email parameters and responds with a User
  addBlock(input:AddBlockInput!):AddBlockPayload
}
`
