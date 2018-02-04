package validators

import "github.com/quorumcontrol/qc3/ownership"

type InsertOwnershipTransaction struct {
	// what is allowing this user to modify
	Capability ownership.Capability
	// who owns what you're modifying
	ParentOwnership string
	// the new ownership
	OwnershipDescription ownership.OwnershipDescription
	// the actual change
	Invocation ownership.Invocation
}


func IsValidOwnershipInsert() {

}
