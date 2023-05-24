package config

const (
	FetchClientData uint32 = iota
	FetchOldData
	ClientId
	ClientData
	ClientExit
	TransferData
	TransferEnd

	ClientServePort  = 8081
	DataTransferPort = 8082
	PingPort         = 8083

	Namespace      = "default"
	EtcdClientMapName = "client-map"

	RedisPort = 7777
)
