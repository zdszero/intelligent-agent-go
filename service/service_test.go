package service

import (
	"fmt"
	"testing"
)

func TestService(t *testing.T) {
	cli := NewK8SClient("")
	fmt.Println(cli.GetNamespaceServices("default"))
}

func TestReadWrite(t *testing.T) {
	cli := NewK8SClient("")
	_ = cli.EtcdPut("client1", "hello world!")
	_ = cli.EtcdPut("client2", "HELLO WORLD!")
	val, _ := cli.EtcdGet("client1")
	fmt.Println("read value:", val)
	val, _ = cli.EtcdGet("client2")
	fmt.Println("read value:", val)
	cli.EtcdDelete("client1")
	val, _ = cli.EtcdGet("client1")
	fmt.Println("read value:", val)
	val, _ = cli.EtcdGet("client2")
	fmt.Println("read value:", val)
}
