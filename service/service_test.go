package service

import (
	"fmt"
	"testing"
)

func TestService(t *testing.T) {
	cli := NewK8SClient("")
	fmt.Println(cli.GetNamespaceServices("default"))
}
