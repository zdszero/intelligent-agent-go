#! /bin/bash

printHelp() {
    echo "Usage: ./run.sh [build|clean|log|ssh]"
}

DEPLOY_NAME="my-agent-deployment"
SERVICE_NAME="my-agent-service"
podname=$(minikube kubectl -- get pods | grep $DEPLOY_NAME | awk '{print $1}')

if [[ "$1" == "build" ]]; then
    echo "Running build operation..."
    go build -o server cmd/server/main.go
    docker build -t my-agent .
    minikube kubectl -- apply -f deployment.yaml
    minikube kubectl -- apply -f service.yaml
elif [[ "$1" == "clean" ]]; then
    echo "Running clean operation..."
    minikube kubectl -- delete service $SERVICE_NAME
    minikube kubectl -- delete deployment $DEPLOY_NAME
elif [[ "$1" == "log" ]]; then
    minikube kubectl -- logs $podname
elif [[ "$1" == "ssh" ]]; then
    minikube kubectl -- exec -it $podname -- /bin/sh
else
    printHelp
fi
