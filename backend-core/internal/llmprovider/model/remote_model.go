package model

// RemoteModel is one entry of a provider's published model list, before it is stored.
type RemoteModel struct {
	ModelRef           string
	DisplayName        string
	Capabilities       []Capability
	ContextWindow      *int
	InputPricePerMTok  *float64
	OutputPricePerMTok *float64
}
