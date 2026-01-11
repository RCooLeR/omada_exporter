package model

type Radio struct {
	RdMode           string  `json:"rdMode"`
	BandWidth        string  `json:"bandWidth"`
	MaxTxRate        int32   `json:"maxTxRate"`
	InterUtilization float64 `json:"interUtil"`
	RxUtilization    float64 `json:"rxUtil"`
	TxUtilization    float64 `json:"txUtil"`
}
