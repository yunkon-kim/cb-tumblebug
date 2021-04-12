#!/bin/bash

#function get_cloud() {
    FILE=../conf.env
    if [ ! -f "$FILE" ]; then
        echo "$FILE does not exist."
        exit
    fi

    FILE=../credentials.conf
    if [ ! -f "$FILE" ]; then
        echo "$FILE does not exist."
        exit
    fi

    TestSetFile=${4:-../testSet.env}
    
    FILE=$TestSetFile
    if [ ! -f "$FILE" ]; then
        echo "$FILE does not exist."
        exit
    fi
	source $TestSetFile
    TestSetFile=${4:-../testSet.env}
    
    FILE=$TestSetFile
    if [ ! -f "$FILE" ]; then
        echo "$FILE does not exist."
        exit
    fi
	source $TestSetFile
    source ../conf.env
    source ../credentials.conf
    AUTH="Authorization: Basic $(echo -n $ApiUsername:$ApiPassword | base64)"

    echo "####################################################################"
    echo "## 0. Get Cloud Connction Config"
    echo "####################################################################"

    CSP=${1}
    REGION=${2:-1}
    POSTFIX=${3:-developer}
    
	source ../common-functions.sh
	getCloudIndex $CSP

    RESTSERVER=localhost

    # for Cloud Connection Config Info
    curl -H "${AUTH}" -sX GET http://$SpiderServer/spider/connectionconfig/${CONN_CONFIG[$INDEX,$REGION]} | json_pp


    # for Cloud Region Info
    curl -H "${AUTH}" -sX GET http://$SpiderServer/spider/region/${RegionName[$INDEX,$REGION]} | json_pp


    # for Cloud Credential Info
    curl -H "${AUTH}" -sX GET http://$SpiderServer/spider/credential/${CredentialName[INDEX]} | json_pp

    
    # for Cloud Driver Info
    curl -H "${AUTH}" -sX GET http://$SpiderServer/spider/driver/${DriverName[INDEX]} | json_pp

#}

#get_cloud
