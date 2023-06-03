package config

const (
	FetchClientData uint32 = iota
	FetchOldData
	SendFreshData
	// client type
	ClientType
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

	RoleSender   = "sender"
	RoleReceiver = "receiver"

	Namespace            = "smart-agent"
	EtcdClientMapName    = "client-map"
	ProxyServicePrefix   = "proxy-service"
	ClusterServicePrefix = "cluster-service"

	RedisPort = 7777
)
