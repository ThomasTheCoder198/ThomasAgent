package dto

// DeleteResponse is the body of every successful DELETE in this domain.
type DeleteResponse struct {
	Deleted bool `json:"deleted"`
}
