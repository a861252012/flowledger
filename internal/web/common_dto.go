package web

type walletMutationResponse struct {
	Restored bool `json:"restored,omitempty"`
	Changed  bool `json:"changed,omitempty"`
}
