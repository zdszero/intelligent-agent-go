### dependencies

- `go 1.20`
- `minikube`

### build & run

```
minikube start
eval $(minikube docker-env)
# show help message using ./run.sh help
# build target docker container and run 3 agents in k8s cluster
./run.sh build 3

# build and run client
go build -o client cmd/client/main.go
# create two clients, cli1 send message to cli2
./client -client cli1 --sendto cli2
./client -client cli2 --recvfrom cli1
# running these programs will display you with a REPL, input .help for help message
```

### workflow

```
client --------> server ---------> prev server
     my client id
     client type
     peer client id
     this cluster ip
     prev cluster ip
                        ---------->
                        fetch old data
                        client id
                        <----------
                        all previous data
       <--------
    all previous data
    transfer end

# if client is sender:
sender ---------> server ---------> peer server --------> receiver
        data
        data
        ...
     peer cluster ip
                        ---------->
                        fresh data
                        my client id
                        all buffered data
                                               ---------->
                                          through connMap[senderId]
                                               all buffered data
    ------------->
        data
                        ------------>
                            data
                                              ------------>
                                                  data
```
