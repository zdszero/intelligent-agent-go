package main

import (
	"testing"
)

func TestDataTransfer(t *testing.T) {
	cli := newAgentClient("dingzf")
	cli.showService()
	cli.connectToService("my-agent-service1")
	cli.sendData("hello")
	cli.sendData("world")
	cli.sendData("end")
	cli.fetchClientData("dingzf")
	cli.connectToService("my-agent-service2")
	cli.fetchClientData("dingzf")
	cli.disconnect()
}
