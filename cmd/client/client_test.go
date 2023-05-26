package main

import (
	"smart-agent/config"
	"testing"
)

func TestDataTransfer(t *testing.T) {
	cli := newAgentClient("dingzf")
	cli.showService()
	cli.connectToService(config.ProxyServicePrefix + "1")
	cli.sendData("hello")
	cli.sendData("world")
	cli.sendData("end")
	cli.fetchClientData("dingzf")
	cli.connectToService(config.ProxyServicePrefix + "2")
	cli.fetchClientData("dingzf")
	cli.disconnect()
}
