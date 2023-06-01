### dependencies

- `go 1.20`
- `minikube`

### workflow

```
myself --------> server ---------> prev server
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

# if myself is sender:
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
