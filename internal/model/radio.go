package model

type Radio struct {
	RdMode        string  `json:"rdMode"`
	BandWidth     string  `json:"bandWidth"`
	MaxTxRate     int32   `json:"maxTxRate"`
	RxUtilization float64 `json:"rxUtili"`
	TxUtilization float64 `json:"txUtili"`
}
