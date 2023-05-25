#! /bin/bash

printHelp() {
    echo "Usage: ./run.sh [build|show|clean|log|ssh]"
}

DEPLOY_NAME="my-agent-deployment"
SERVICE_NAME="my-agent-service"
NAMESPACE="smart-agent"

if [[ "$1" == "build" ]]; then
    echo "Running build operation..."
    eval $(minikube docker-env)
    go build -o server cmd/server/main.go
    docker build -t my-agent .
    num=$2
    for (( i = 1; i <= $num; i++ )); do
        printf "build ${SERVICE_NAME}%d\n" $i
        deploy_temp=$(mktemp)
        sed "s/${DEPLOY_NAME}/\0${i}/" deployment.yaml > $deploy_temp
        minikube kubectl -- apply -f $deploy_temp
        service_temp=$(mktemp)
        sed "s/${SERVICE_NAME}/\0${i}/" service.yaml > $service_temp
        minikube kubectl -- apply -f $service_temp
        rm $deploy_temp
        rm $service_temp
    done
elif [[ "$1" == "show" ]]; then
    echo "Services":
    minikube kubectl -- get pods -n ${NAMESPACE}
    echo "Deployments":
    minikube kubectl -- get deployment -n ${NAMESPACE}
elif [[ "$1" == "clean" ]]; then
    echo "Running clean operation..."
    running_services=$(minikube kubectl -- get svc -n $NAMESPACE | grep 'my-agent-service' | awk '{print $1}')
    while IFS= read -r service; do
        deploy=$(echo $service | sed 's/service/deployment/')
        minikube kubectl -- delete service $service -n $NAMESPACE
        minikube kubectl -- delete deployment $deploy -n $NAMESPACE
    done <<< $running_services
elif [[ "$1" == "log" ]]; then
    [[ $# < 2 ]] && exit 0
    podname=$(minikube kubectl -- get pods -n ${NAMESPACE} | grep "${DEPLOY_NAME}${2}" | awk '{print $1}')
    # echo $podname
    minikube kubectl -- logs $podname -n ${NAMESPACE}
elif [[ "$1" == "ssh" ]]; then
    [[ $# < 2 ]] && exit 0
    podname=$(minikube kubectl -- get pods -n ${NAMESPACE} | grep "${DEPLOY_NAME}${2}" | awk '{print $1}')
    minikube kubectl -- exec -it $podname -- /bin/sh
else
    printHelp
fi
