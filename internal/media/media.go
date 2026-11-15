package media

type State struct {
	Available   bool   `json:"available"`
	Player      string `json:"player,omitempty"`
	Title       string `json:"title,omitempty"`
	Artist      string `json:"artist,omitempty"`
	Playing     bool   `json:"playing"`
	CanToggle   bool   `json:"canToggle"`
	CanNext     bool   `json:"canNext"`
	CanPrevious bool   `json:"canPrevious"`
}
