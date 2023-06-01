#! /bin/bash

printHelp() {
    echo 'Usage: ./run.sh [build|show|clean|log|ssh]'
    echo '  ./run.sh build n'
    echo '      build and run n proxy agents in k8s'
    echo '  ./run.sh show'
    echo '      display all services and deployments in target namespace'
    echo '  ./run.sh clean'
    echo '      clean all services and deployments in k8s'
    echo '  ./run.sh log i'
    echo "      display the ith agent's log"
    echo '  ./run.sh ssh i'
    echo "      use ssh to connect into the ith agent"
}

DEPLOY="proxy-deployment"
PROXY_SERVICE="proxy-service"
CLUSTER_SERVICE="cluster-service"
SELECTOR_APP="proxy-app"
NAMESPACE="smart-agent"

if [[ "$1" == "build" ]]; then
    echo "Running build operation..."
    eval $(minikube docker-env)
    go build -o server cmd/server/main.go
    docker build -t my-agent .
    num=$2
    for (( i = 1; i <= $num; i++ )); do
        printf "build ${DEPLOY}%d\n" $i
        deploy_temp=$(mktemp)
        sed "s/${DEPLOY}/\0${i}/" deployment.yaml > $deploy_temp
        sed -i "s/${PROXY_SERVICE}/\0${i}/" $deploy_temp
        sed -i "s/${CLUSTER_SERVICE}/\0${i}/" $deploy_temp
        sed -i "s/${SELECTOR_APP}/\0${i}/" $deploy_temp
        minikube kubectl -- apply -f $deploy_temp
        rm $deploy_temp
    done
elif [[ "$1" == "show" ]]; then
    echo "Services":
    minikube kubectl -- get svc -n ${NAMESPACE}
    echo "Deployments":
    minikube kubectl -- get deployment -n ${NAMESPACE}
elif [[ "$1" == "clean" ]]; then
    echo "Running clean operation..."
    running_services=$(minikube kubectl -- get svc -n $NAMESPACE | grep $PROXY_SERVICE | awk '{print $1}')
    while IFS= read -r service; do
        deploy=$(echo $service | sed 's/service/deployment/')
        minikube kubectl -- delete service $service -n $NAMESPACE
        minikube kubectl -- delete deployment $deploy -n $NAMESPACE
    done <<< $running_services
    running_services=$(minikube kubectl -- get svc -n $NAMESPACE | grep $CLUSTER_SERVICE | awk '{print $1}')
    while IFS= read -r service; do
        minikube kubectl -- delete service $service -n $NAMESPACE
    done <<< $running_services
elif [[ "$1" == "log" ]]; then
    [[ $# < 2 ]] && exit 0
    podname=$(minikube kubectl -- get pods -n ${NAMESPACE} | grep "${DEPLOY}${2}" | awk '{print $1}')
    # echo $podname
    minikube kubectl -- logs -f $podname -n ${NAMESPACE}
elif [[ "$1" == "ssh" ]]; then
    [[ $# < 2 ]] && exit 0
    podname=$(minikube kubectl -- get pods -n ${NAMESPACE} | grep "${DEPLOY}${2}" | awk '{print $1}')
    minikube kubectl -- exec -it $podname -n ${NAMESPACE} -- /bin/sh
else
    printHelp
fi
