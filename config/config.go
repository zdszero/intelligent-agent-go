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

	Namespace            = "smart-agent"
	EtcdClientMapName    = "client-map"
	KubernetesIp         = "192.168.49.2"
	ProxyServicePrefix   = "proxy-service"
	ClusterServicePrefix = "cluster-service"

	RedisPort = 7777
)
