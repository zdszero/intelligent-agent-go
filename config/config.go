package config

const (
	FetchClientData uint32 = iota
	ClientId
	TransferFinished
	ClientData
	ClientExit
	TransferData
	TransferEnd
	InvalidClientId

	ClientServePort  = 8081
	DataTransferPort = 8082
	PingPort         = 8083

	Namespace         = "default"
	EtcdClientMapName = "client-map"
	KubernetesIp      = "192.168.49.2"

	RedisPort = 7777
)
