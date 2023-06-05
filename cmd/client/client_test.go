package main

import (
	"fmt"
	"smart-agent/config"
	"testing"
	"time"
)

func TestSimpleLocal(t *testing.T) {
	receiver := newAgentClient("receiver", "", 0)
	receiver.setReceiver([]string{"sender"})
	receiver.debugConnect("172.16.1.62", config.ClientServePort)
	fmt.Println("receiver connect finished")
	go func() {
		receiver.roleTask()
	}()

	sender := newAgentClient("sender", "", 0)
	sender.setSender("receiver")
	sender.debugConnect("172.16.1.147", config.ClientServePort)
	sender.roleTask()
	sender.sendData("nihao")
	sender.sendData("test")
	sender.sendData("after")
	time.Sleep(time.Second * 3)
	sender.disconnect()
}

func TestSimpleCluster(t *testing.T) {
	receiver := newAgentClient("receiver", "", 0)
	receiver.updateServerInfo()
	receiver.setReceiver([]string{"sender"})
	receiver.etcdCleanup()
	receiver.connectToService(config.ProxyServicePrefix + "1")
	fmt.Println("receiver connect finished")
	go func() {
		receiver.roleTask()
	}()

	sender := newAgentClient("sender", "", 0)
	sender.setSender("receiver")
	sender.updateServerInfo()
	sender.etcdCleanup()
	sender.connectToService(config.ProxyServicePrefix + "2")
	sender.roleTask()
	sender.sendData("nihao")
	sender.sendData("test")
	sender.sendData("after")
	time.Sleep(time.Second * 3)
	sender.disconnect()
}

func TestSameAgent(t *testing.T) {
	receiver := newAgentClient("receiver", "", 0)
	receiver.setReceiver([]string{"sender"})
	receiver.updateServerInfo()
	receiver.connectToService(config.ProxyServicePrefix + "1")
	fmt.Println("receiver connect finished")
	go func() {
		receiver.roleTask()
	}()

	sender := newAgentClient("sender", "", 0)
	sender.setSender("receiver")
	sender.updateServerInfo()
	sender.connectToService(config.ProxyServicePrefix + "1")
	sender.roleTask()
	sender.sendData("nihao")
	sender.sendData("test")
	sender.sendData("after")
	time.Sleep(time.Second * 3)
	sender.disconnect()
}

func TestReceiverJoinLater(t *testing.T) {
	senderCh := make(chan bool)
	receiverCh := make(chan bool)
	sender := newAgentClient("sender", "", 0)
	sender.updateServerInfo()
	sender.setSender("receiver")
	receiver := newAgentClient("receiver", "", 0)
	receiver.updateServerInfo()
	receiver.setReceiver([]string{"sender"})
	sender.etcdCleanup()
	receiver.etcdCleanup()
	go func() {
		sender.connectToService(config.ProxyServicePrefix + "1")
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
		fmt.Println("receiver connects ...")
		receiver.roleTask()
		time.Sleep(time.Second * 4)
		receiverCh <- true
	}()
	<-senderCh
	<-receiverCh
}

func TestReceiverJoinAfterSenderExit(t *testing.T) {
	senderCh := make(chan bool)
	receiverCh := make(chan bool)
	sender := newAgentClient("sender", "", 0)
	sender.updateServerInfo()
	sender.setSender("receiver")
	receiver := newAgentClient("receiver", "", 0)
	receiver.updateServerInfo()
	receiver.setReceiver([]string{"sender"})
	sender.etcdCleanup()
	receiver.etcdCleanup()
	go func() {
		sender.connectToService(config.ProxyServicePrefix + "1")
		sender.roleTask()
		sender.sendData("nihao")
		sender.sendData("test")
		sender.sendData("after")
		sender.disconnect()
		senderCh <- true
	}()
	go func() {
		time.Sleep(time.Second * 2)
		receiver.connectToService(config.ProxyServicePrefix + "2")
		receiver.roleTask()
		receiverCh <- true
	}()
	<-senderCh
	<-receiverCh
}

func TestPriority(t *testing.T) {
	sender1 := newAgentClient("sender1", "", 80)
	sender1.updateServerInfo()
	sender1.setSender("receiver")

	sender2 := newAgentClient("sender2", "", 100)
	sender2.updateServerInfo()
	sender2.setSender("receiver")

	receiver := newAgentClient("receiver", "", 0)
	receiver.updateServerInfo()
	receiver.setReceiver([]string{"sender1", "sender2"})

	sender1.etcdCleanup()
	sender2.etcdCleanup()
	receiver.etcdCleanup()

	ch1 := make(chan bool)
	ch2 := make(chan bool)
	ch3 := make(chan bool)

	receiver.connectToService(config.ProxyServicePrefix + "2")
	go func() {
		receiver.roleTask()
		ch3 <- true
	}()

	go func() {
		sender1.connectToService(config.ProxyServicePrefix + "1")
		sender1.roleTask()
		sender1.sendData("nihao_sender1")
		sender1.sendData("haha_sender1")
		time.Sleep(3 * time.Second)
		sender1.sendData("end_sender1")
		sender1.disconnect()
		ch1 <- true
	}()

	go func() {
		sender2.connectToService(config.ProxyServicePrefix + "1")
		sender2.roleTask()
		time.Sleep(1 * time.Second)
		sender2.sendData("nihao_sender2")
		sender2.sendData("haha_sender2")
		sender2.sendData("end_sender2")
		sender2.disconnect()
		ch2 <- true
	}()

	<-ch1
	<-ch2
	<-ch3
}
