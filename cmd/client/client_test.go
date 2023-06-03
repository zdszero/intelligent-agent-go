package main

import (
	"fmt"
	"smart-agent/config"
	"testing"
	"time"
)

func TestSimple(t *testing.T) {
		receiver := newAgentClient("receiver", "")
		receiver.setRole("receiver", "sender")
		receiver.debugConnect("172.16.1.62", config.ClientServePort)
		fmt.Println("receiver connect finished")
		go func() {
			receiver.roleTask()
		}()

		sender := newAgentClient("sender", "")
		sender.setRole("sender", "receiver")
		sender.debugConnect("172.16.1.147", config.ClientServePort)
		sender.roleTask()
		sender.sendData("nihao")
		sender.sendData("test")
		sender.sendData("after")
		time.Sleep(time.Second * 3)
		sender.disconnect()
}

func TestSendRecv(t *testing.T) {
	senderCh := make(chan bool)
	receiverCh := make(chan bool)
	sender := newAgentClient("sender", "")
	sender.setRole("sender", "receiver")
	receiver := newAgentClient("receiver", "")
	receiver.setRole("receiver", "sender")
	sender.etcdCleanup()
	receiver.etcdCleanup()
	go func() {
		sender.debugConnect("172.16.1.147", config.ClientServePort)
		fmt.Println("sender connect finished")
		sender.roleTask()
		sender.sendData("nihao")
		sender.sendData("test")
		time.Sleep(time.Second * 2)
		sender.sendData("after")
		sender.disconnect()
		senderCh <- true
	}()
	go func() {
		time.Sleep(time.Second * 1)
		receiver.debugConnect("172.16.1.62", config.ClientServePort)
		fmt.Println("receiver connect finished")
		receiver.roleTask()
		time.Sleep(time.Second * 4)
		receiverCh <- true
	}()
	<-senderCh
	<-receiverCh
}

func TestSendRecvCluster(t *testing.T) {
	senderCh := make(chan bool)
	receiverCh := make(chan bool)
	sender := newAgentClient("sender", "")
	sender.updateServerInfo()
	sender.setRole(config.RoleSender, "receiver")
	receiver := newAgentClient("receiver", "")
	receiver.updateServerInfo()
	receiver.setRole(config.RoleReceiver, "sender")
	sender.etcdCleanup()
	receiver.etcdCleanup()
	go func() {
		sender.connectToService(config.ProxyServicePrefix + "1")
		fmt.Println("sender connect finished")
		sender.roleTask()
		sender.sendData("nihao")
		sender.sendData("test")
		time.Sleep(time.Second * 2)
		sender.sendData("after")
		sender.disconnect()
		senderCh <- true
	}()
	go func() {
		time.Sleep(time.Second * 1)
		receiver.connectToService(config.ProxyServicePrefix + "2")
		fmt.Println("receiver connect finished")
		receiver.roleTask()
		time.Sleep(time.Second * 4)
		receiverCh <- true
	}()
	<-senderCh
	<-receiverCh

}
