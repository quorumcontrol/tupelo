package ownership


type OwnershipDescription struct {
	Context string `json:"@context"`
	Id string `json:"id"`
	MinimumSignatures int `json:"minimumSignatures"`
	Owners []string `json:"owners"`
	Did string `json:"did"`
	// A forward-linked list of all children ownerships
	Children []string `json:"children"`
}