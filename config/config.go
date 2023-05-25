package config

const (
	FetchClientData uint32 = iota
	// client settings
	ClientId
	ClusterIp
	TransferFinished
	ClientData
	ClientExit
	// transfer settings
	TransferData
	TransferEnd

	ClientServePort  = 8081
	DataTransferPort = 8082
	PingPort         = 8083

	Namespace         = "default"
	EtcdClientMapName = "client-map"
	KubernetesIp      = "192.168.49.2"

	RedisPort = 7777
)
