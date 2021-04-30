#!/bin/bash

#function list_sshKey() {


    TestSetFile=${4:-../testSet.env}
    
    FILE=$TestSetFile
    if [ ! -f "$FILE" ]; then
        echo "$FILE does not exist."
        exit
    fi
	source $TestSetFile
    source ../conf.env
    AUTH="Authorization: Basic $(echo -n $ApiUsername:$ApiPassword | base64)"

    echo "####################################################################"
    echo "## 5. sshKey: List"
    echo "####################################################################"

    curl -H "${AUTH}" -sX GET http://$TumblebugServer/tumblebug/ns/$NSID/resources/sshKey | jq ''
    echo ""
#}

#list_sshKey