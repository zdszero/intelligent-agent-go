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
CONFIGMAP="client-map"
K="minikube kubectl --"

createNsCm() {
    ns_cnt=$($K get ns | grep ${NAMESPACE} | wc -l)
    if [[ $ns_cnt -eq 0 ]]; then
        echo "create namespace ${NAMESPACE}"
        $K create ns ${NAMESPACE}
    fi
    cm_cnt=$($K get configmap -n ${NAMESPACE} | grep ${CONFIGMAP} | wc -l)
    if [[ $cm_cnt -eq 0 ]]; then
        echo "create configmap ${CONFIGMAP}"
        $K create configmap ${CONFIGMAP} --from-file=configmap.yaml -n ${NAMESPACE}
    fi  
}

if [[ "$1" == "build" ]]; then
    echo "Running build operation..."
    eval $(minikube docker-env)
    createNsCm
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
        $K apply -f $deploy_temp
        rm $deploy_temp
    done
elif [[ "$1" == "show" ]]; then
    echo "Services":
    $K get svc -n ${NAMESPACE}
    echo "Deployments":
    $K get deployment -n ${NAMESPACE}
elif [[ "$1" == "clean" ]]; then
    echo "Running clean operation..."
    running_services=$($K get svc -n $NAMESPACE | grep $PROXY_SERVICE | awk '{print $1}')
    while IFS= read -r service; do
        deploy=$(echo $service | sed 's/service/deployment/')
        $K delete service $service -n $NAMESPACE
        $K delete deployment $deploy -n $NAMESPACE
    done <<< $running_services
    running_services=$($K get svc -n $NAMESPACE | grep $CLUSTER_SERVICE | awk '{print $1}')
    while IFS= read -r service; do
        $K delete service $service -n $NAMESPACE
    done <<< $running_services
elif [[ "$1" == "log" ]]; then
    [[ $# < 2 ]] && exit 0
    podname=$($K get pods -n ${NAMESPACE} | grep "${DEPLOY}${2}" | awk '{print $1}')
    # echo $podname
    $K logs -f $podname -n ${NAMESPACE}
elif [[ "$1" == "ssh" ]]; then
    [[ $# < 2 ]] && exit 0
    podname=$($K get pods -n ${NAMESPACE} | grep "${DEPLOY}${2}" | awk '{print $1}')
    $K exec -it $podname -n ${NAMESPACE} -- /bin/sh
else
    printHelp
fi
