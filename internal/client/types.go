package client

type State struct {
	Status       string  `json:"status"` // "play","pause","stop"
	Title        string  `json:"title"`
	Artist       string  `json:"artist"`
	Album        string  `json:"album"`
	Seek         int64   `json:"seek"`
	Duration     float64 `json:"duration"`
	Volume       int     `json:"volume"`
	Repeat       bool    `json:"repeat"`
	Random       bool    `json:"random"`
	Consume      bool    `json:"consume"`
	VolumioVer   string  `json:"volumio_version"`
	Service      string  `json:"service"`
	TrackType    string  `json:"trackType"`
	Samplerate   string  `json:"samplerate"`
	Bitdepth     string  `json:"bitdepth"`
	Channels     int     `json:"channels"`
	Updated      string  `json:"updated"`
	DisableState bool    `json:"disableUiControls"`
}
