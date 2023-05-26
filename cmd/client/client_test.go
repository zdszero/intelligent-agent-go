package main

import (
	"smart-agent/config"
	"testing"
)

func TestDataTransfer(t *testing.T) {
	cli := newAgentClient("dingzf")
	cli.connectToService(config.ProxyServicePrefix + "1")
	cli.sendData("hello")
	cli.sendData("world")
	cli.sendData("end")
	cli.fetchClientData("dingzf")
	cli.connectToService(config.ProxyServicePrefix + "2")
	cli.fetchClientData("dingzf")
	cli.disconnect()
}

func TestFetchOtherData(t *testing.T) {
	cli := newAgentClient("dingzf")
	cli.connectToService(config.ProxyServicePrefix + "1")
	cli.sendData("dingzf1")
	cli.sendData("dingzf2")
	cli2 := newAgentClient("chenjw")
	cli2.connectToService(config.ProxyServicePrefix + "2")
	cli2.sendData("chenjw1")
	cli2.sendData("chenjw2")
	cli.fetchClientData("chenjw")
	cli2.fetchClientData("dingzf")
	cli.disconnect()
	cli2.disconnect()
}
